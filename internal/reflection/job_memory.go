package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

// SessionSource is the narrow interface the memory job needs from the
// session store. The real *session.Store satisfies it.
type SessionSource interface {
	ListAll() ([]session.Session, error)
	TranscriptPath(id string) string
}

// MemoryJob runs the "memory cleanup" half of reflection. It is the
// batch form of the per-turn derivation+compilation logic that used to
// live in internal/tarsserver/chat_memory_hook.go. Moving the work here
// takes LLM calls off the per-turn hot path and lets operators tune
// lookback windows and turn caps via config.
//
// For each session updated within Lookback, the job:
//
//  1. Reads the last MaxTurnsPerSession transcript messages;
//  2. Pairs consecutive user/assistant messages into turns;
//  3. For each turn, derives 0..N auto experiences via keyword rules;
//  4. For each turn that clears the knowledge-base gate, calls the LLM
//     to compile structured knowledge and applies the diff.
//
// The job is idempotent at the experience level: appendExperienceIfNew
// dedupes against existing entries by summary+category match.
//
// The knowledge-compilation call uses the llm.RoleReflectionMemory role,
// which operators can map to the light tier via llm_role_reflection_memory
// to keep nightly runs cheap.
type MemoryJob struct {
	WorkspaceDir       string
	Sessions           SessionSource
	Router             llm.Router
	Lookback           time.Duration
	MaxTurnsPerSession int
	Now                func() time.Time
}

// Name implements Job.
func (m *MemoryJob) Name() string { return "memory" }

// Run implements Job. Errors accumulate into result.Details["errors"];
// the job only returns a non-nil error when it cannot even read the
// session list.
func (m *MemoryJob) Run(ctx context.Context) (JobResult, error) {
	if m == nil {
		return JobResult{Name: "memory"}, nil
	}
	now := m.now()
	lookback := m.Lookback
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	maxTurns := m.MaxTurnsPerSession
	if maxTurns <= 0 {
		maxTurns = 20
	}

	if m.Sessions == nil {
		return JobResult{Name: "memory", Success: false, Err: "no session source"}, nil
	}
	sessions, err := m.Sessions.ListAll()
	if err != nil {
		return JobResult{Name: "memory"}, fmt.Errorf("list sessions: %w", err)
	}

	cutoff := now.Add(-lookback)
	var (
		sessionsScanned  int
		turnsProcessed   int
		experiencesAdded int
		kbUpdates        int
		errs             []string
	)

	for _, sess := range sessions {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err.Error())
			break
		}
		if sess.UpdatedAt.Before(cutoff) {
			continue
		}
		sessionsScanned++

		path := m.Sessions.TranscriptPath(sess.ID)
		messages, err := session.ReadMessages(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read %s: %s", sess.ID, err.Error()))
			continue
		}
		// Take the last maxTurns*2 messages (rough cap — some turns are
		// tool/system and won't pair).
		if len(messages) > maxTurns*2 {
			messages = messages[len(messages)-maxTurns*2:]
		}

		turns := pairTurns(messages)
		if len(turns) > maxTurns {
			turns = turns[len(turns)-maxTurns:]
		}

		for _, t := range turns {
			turnsProcessed++

			expCount := m.processTurnExperiences(sess.ID, t, now)
			experiencesAdded += expCount

			if m.compileKnowledge(ctx, sess.ID, t, now) {
				kbUpdates++
			}
		}
	}

	result := JobResult{
		Name:    "memory",
		Success: true,
		Summary: fmt.Sprintf("scanned %d sessions, %d turns, +%d experiences, %d kb updates", sessionsScanned, turnsProcessed, experiencesAdded, kbUpdates),
		Details: map[string]any{
			"sessions_scanned":  sessionsScanned,
			"turns_processed":   turnsProcessed,
			"experiences_added": experiencesAdded,
			"kb_updates":        kbUpdates,
			"lookback_seconds":  int64(lookback.Seconds()),
		},
		Changed: experiencesAdded > 0 || kbUpdates > 0,
	}
	if len(errs) > 0 {
		result.Details["errors"] = errs
	}
	return result, nil
}

func (m *MemoryJob) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// turn represents one user→assistant exchange. Tool/system messages
// between them are ignored for derivation purposes.
type turn struct {
	UserMessage      string
	AssistantMessage string
	At               time.Time
}

func pairTurns(messages []session.Message) []turn {
	var turns []turn
	var pending *turn
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "user":
			if pending != nil && pending.AssistantMessage == "" {
				// Previous user message had no assistant reply; drop it.
				pending = nil
			}
			pending = &turn{UserMessage: msg.Content, At: msg.Timestamp}
		case "assistant":
			if pending != nil {
				pending.AssistantMessage = msg.Content
				if pending.At.IsZero() {
					pending.At = msg.Timestamp
				}
				turns = append(turns, *pending)
				pending = nil
			}
		}
	}
	return turns
}

func (m *MemoryJob) processTurnExperiences(sessionID string, t turn, now time.Time) int {
	count := 0
	for _, exp := range deriveTurnExperiences(sessionID, t, now) {
		if appendExperienceIfNew(m.WorkspaceDir, exp) {
			count++
		}
	}
	return count
}

func (m *MemoryJob) compileKnowledge(ctx context.Context, sessionID string, t turn, now time.Time) bool {
	if m.Router == nil || !shouldCompileKnowledge(t) {
		return false
	}
	client, _, err := m.Router.ClientFor(llm.RoleReflectionMemory)
	if err != nil {
		return false
	}
	store := memory.NewKnowledgeStore(m.WorkspaceDir, nil)
	existing, err := store.List(memory.KnowledgeListOptions{Limit: 12})
	if err != nil {
		return false
	}

	var refs strings.Builder
	for _, item := range existing {
		fmt.Fprintf(&refs, "- slug=%s | title=%s | kind=%s | summary=%s\n",
			item.Slug, item.Title, item.Kind, trimText(item.Summary, 120))
	}
	if refs.Len() == 0 {
		refs.WriteString("- (none)\n")
	}

	userPrompt := fmt.Sprintf(
		"Compile durable knowledge from one chat turn into a wiki-style knowledge base.\n"+
			"Return strict JSON with shape {\"notes\":[{\"slug\":\"lower-kebab-case\",\"title\":\"...\",\"kind\":\"preference|fact|decision|habit|workflow|topic|project_note|person|note\",\"summary\":\"...\",\"body\":\"...\",\"tags\":[\"...\"],\"aliases\":[\"...\"],\"links\":[{\"target\":\"slug\",\"relation\":\"related_to|depends_on|supports|part_of|contradicts\"}],\"source_session\":\"...\"}],\"edges\":[{\"source\":\"slug\",\"target\":\"slug\",\"relation\":\"...\"}]}.\n"+
			"Only keep durable knowledge worth reusing after chat/session reset.\n"+
			"Reuse existing slugs when the note already exists. Return empty arrays when nothing durable should be added.\n\n"+
			"Current note refs:\n%s\n"+
			"Session: %s\nUser: %s\nAssistant: %s",
		refs.String(),
		strings.TrimSpace(sessionID),
		strings.TrimSpace(t.UserMessage),
		strings.TrimSpace(t.AssistantMessage),
	)

	resp, err := client.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You maintain a structured markdown knowledge base. Return strict JSON only."},
		{Role: "user", Content: userPrompt},
	}, llm.ChatOptions{})
	if err != nil {
		return false
	}
	raw := strings.TrimSpace(resp.Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}

	var update memory.KnowledgeUpdate
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		return false
	}
	if len(update.Notes) == 0 && len(update.Edges) == 0 {
		return false
	}
	for i := range update.Notes {
		if strings.TrimSpace(update.Notes[i].SourceSession) == "" {
			update.Notes[i].SourceSession = strings.TrimSpace(sessionID)
		}
		update.Notes[i].UpdatedAt = now.UTC()
	}
	if err := store.ApplyUpdate(update, now.UTC()); err != nil {
		return false
	}
	return true
}
