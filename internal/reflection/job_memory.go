package reflection

import (
	"context"
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
//  3. For each turn, derives 0..N auto memory candidates via keyword rules;
//  4. For each turn that clears the knowledge-base gate, calls the LLM
//     to compile structured knowledge and applies the diff.
//
// The job is idempotent at the candidate level: memory inbox appends
// dedupe against existing candidates by stable ID and summary.
//
// The knowledge-compilation call uses the llm.RoleReflectionMemory role,
// which operators can map to the light tier via llm_role_reflection_memory
// to keep nightly runs cheap.
type MemoryJob struct {
	WorkspaceDir       string
	Backend            memory.Backend
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
		sessionsScanned int
		turnsProcessed  int
		candidatesAdded int
		errs            []string
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

			candidateCount := m.processTurnMemoryCandidates(ctx, sess.ID, t, now)
			candidatesAdded += candidateCount
		}
	}

	result := JobResult{
		Name:    "memory",
		Success: true,
		Summary: fmt.Sprintf("scanned %d sessions, %d turns, +%d memory candidates", sessionsScanned, turnsProcessed, candidatesAdded),
		Details: map[string]any{
			"sessions_scanned":  sessionsScanned,
			"turns_processed":   turnsProcessed,
			"candidates_added":  candidatesAdded,
			"experiences_added": 0,
			"lookback_seconds":  int64(lookback.Seconds()),
		},
		Changed: candidatesAdded > 0,
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
	UserIndex        int
	AssistantIndex   int
}

func pairTurns(messages []session.Message) []turn {
	var turns []turn
	var pending *turn
	for idx, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "user":
			if pending != nil && pending.AssistantMessage == "" {
				// Previous user message had no assistant reply; drop it.
				pending = nil
			}
			pending = &turn{UserMessage: msg.Content, At: msg.Timestamp, UserIndex: idx}
		case "assistant":
			if pending != nil {
				pending.AssistantMessage = msg.Content
				pending.AssistantIndex = idx
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

func (m *MemoryJob) processTurnMemoryCandidates(ctx context.Context, sessionID string, t turn, now time.Time) int {
	count := 0
	for _, exp := range deriveTurnExperiences(sessionID, t, now) {
		if enqueueMemoryCandidateIfNew(ctx, m.WorkspaceDir, m.backend(), exp, t, now) {
			count++
		}
	}
	return count
}

func (m *MemoryJob) backend() memory.Backend {
	if m != nil && m.Backend != nil {
		return m.Backend
	}
	if m == nil {
		return nil
	}
	return memory.NewFileBackend(m.WorkspaceDir, nil)
}

func enqueueMemoryCandidateIfNew(
	ctx context.Context,
	workspaceDir string,
	backend memory.Backend,
	exp memory.Experience,
	t turn,
	now time.Time,
) bool {
	if strings.TrimSpace(workspaceDir) == "" || strings.TrimSpace(exp.Summary) == "" {
		return false
	}
	candidate := memory.MemoryCandidate{
		Category:      exp.Category,
		Summary:       exp.Summary,
		Tags:          exp.Tags,
		SourceSession: exp.SourceSession,
		Importance:    exp.Importance,
		Auto:          exp.Auto,
		CreatedAt:     now.UTC(),
		UpdatedAt:     now.UTC(),
		Provenance: memory.MemoryCandidateProvenance{
			Source:        "reflection",
			SessionID:     strings.TrimSpace(exp.SourceSession),
			MessageRange:  t.messageRange(),
			SourceSummary: trimText(t.UserMessage+" "+t.AssistantMessage, 220),
			ExtractedAt:   now.UTC(),
		},
	}
	_, added, err := memory.AppendInboxCandidateIfNew(ctx, workspaceDir, backend, candidate)
	return err == nil && added
}

func (t turn) messageRange() string {
	if t.UserIndex < 0 || t.AssistantIndex < 0 {
		return ""
	}
	return fmt.Sprintf("messages:%d-%d", t.UserIndex, t.AssistantIndex)
}
