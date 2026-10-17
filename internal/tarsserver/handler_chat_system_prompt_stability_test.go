package tarsserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Every provider caches by prefix, and every client puts the system prompt
// first. A section that appears on one turn only, or a number that moves
// each turn, cuts the cached prefix at that byte for the whole history behind
// it. Two turns of one session — the first invoking a /command and a /skill,
// the second mentioning a subagent after an auto-continue spent goal budget
// and the critic queued feedback — must send the same system message, and
// the per-turn text must still reach the model on the user turn.
func TestChatAPI_SystemPromptIsByteIdenticalAcrossTurns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("stable prompt")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	writeStabilityFixtureFile(t, filepath.Join(sess.CurrentDir, ".tars", "commands", "tidy.md"),
		"---\ndescription: tidy command\n---\nTidy only the files mentioned by the user.\n")
	writeStabilityFixtureFile(t, filepath.Join(sess.CurrentDir, ".tars", "skills", "refactor", "SKILL.md"),
		"---\nname: refactor\ndescription: refactor helper\nuser_invocable: true\n---\n\nrefactor body\n")
	if _, err := store.SetGoal(sess.ID, &session.SessionGoal{
		Description:      "ship the feature",
		CreatedAt:        time.Now().UTC(),
		MaxAutoContinues: 5,
		Status:           session.SessionGoalStatusActive,
	}); err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if _, err := store.SetCritic(sess.ID, &session.SessionCritic{Enabled: true}); err != nil {
		t.Fatalf("set critic: %v", err)
	}

	client := &mockLLMClient{
		response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}},
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, client, nil, zerolog.Nop(), 8, nil, "",
		chatToolingOptions{AgentRuntime: newChatMentionTestRuntime(t, root, store)},
	)

	first := postStabilityTurn(t, handler, client, map[string]any{
		"session_id": sess.ID,
		"message":    "/tidy the auth package",
	})
	if _, err := store.UpdateGoalProgress(sess.ID, func(g *session.SessionGoal) *session.SessionGoal {
		g.AutoContinueCount = 2
		g.Status = session.SessionGoalStatusActive
		return g
	}); err != nil {
		t.Fatalf("spend goal budget: %v", err)
	}
	if _, err := store.UpdateCriticProgress(sess.ID, func(c *session.SessionCritic) *session.SessionCritic {
		c.Enabled = true
		c.PendingFeedback = "critic says: cite the file you changed"
		return c
	}); err != nil {
		t.Fatalf("queue critic feedback: %v", err)
	}
	second := postStabilityTurn(t, handler, client, map[string]any{
		"session_id": sess.ID,
		"message":    "ask @researcher to inspect the auth flow",
		"subagent_mentions": []chatSubagentMentionRequest{
			{Name: "researcher", Token: "@researcher"},
		},
	})
	third := postStabilityTurn(t, handler, client, map[string]any{
		"session_id": sess.ID,
		"message":    "/refactor the session store",
	})

	for i, turn := range [][]llm.ChatMessage{second, third} {
		if turn[0].Content != first[0].Content {
			t.Fatalf("turn %d changed the system prompt at %s", i+2, firstDifference(first[0].Content, turn[0].Content))
		}
	}
	for _, turn := range [][]llm.ChatMessage{first, second, third} {
		for _, msg := range turn[1:] {
			if msg.Role == "system" {
				t.Fatalf("no system message may follow the stable prompt: %+v", msg)
			}
		}
	}

	for _, want := range []string{"## Active Session Goal", "ship the feature", "## Critic agent (active)"} {
		if !strings.Contains(first[0].Content, want) {
			t.Fatalf("stable session state %q must stay in the system prompt: %q", want, first[0].Content)
		}
	}
	checkTurnContext(t, first, "## Invoked Command", "/tidy", "auto-continue budget remaining: 5/5")
	checkTurnContext(t, second, "## Mentioned Subagents", "researcher", "auto-continue budget remaining: 3/5", "critic says: cite the file you changed")
	checkTurnContext(t, third, "## Invoked Skill", "/refactor")

	// The transcript keeps the user's words only, so the next turn replays
	// them without this turn's context and the prefix keeps matching.
	history := second[1 : len(second)-1]
	if got := history[0].Content; got != "/tidy the auth package" {
		t.Fatalf("replayed user turn must not carry its turn context: %q", got)
	}
}

func firstDifference(a, b string) string {
	n := min(len(a), len(b))
	at := n
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			at = i
			break
		}
	}
	from := max(at-80, 0)
	return fmt.Sprintf("byte %d:\nfirst=%q\nlater=%q", at, a[from:min(at+120, len(a))], b[from:min(at+120, len(b))])
}

func checkTurnContext(t *testing.T, msgs []llm.ChatMessage, wants ...string) {
	t.Helper()
	user := msgs[len(msgs)-1]
	if user.Role != "user" {
		t.Fatalf("expected the user turn last, got %+v", user)
	}
	i := strings.LastIndex(user.Content, turnContextOpen)
	if i < 0 {
		t.Fatalf("expected a turn-context block on the user turn: %q", user.Content)
	}
	block := user.Content[i:]
	for _, want := range wants {
		if !strings.Contains(block, want) {
			t.Fatalf("turn context must carry %q: %q", want, block)
		}
	}
}

func postStabilityTurn(t *testing.T, handler http.Handler, client *mockLLMClient, body map[string]any) []llm.ChatMessage {
	t.Helper()
	client.mu.Lock()
	before := len(client.seenMessages)
	client.mu.Unlock()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	// The goal judge and critic reviewer call the same client with their own
	// prompts; the chat turn is the call that ends with this user message.
	message, _ := body["message"].(string)
	for _, msgs := range client.seenMessages[before:] {
		last := msgs[len(msgs)-1]
		if last.Role == "user" && strings.HasPrefix(last.Content, message) {
			return msgs
		}
	}
	t.Fatalf("no chat turn call for %q among %d calls", message, len(client.seenMessages)-before)
	return nil
}

func writeStabilityFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
