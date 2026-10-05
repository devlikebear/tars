package tarsserver

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/goal"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// stubGoalRouter satisfies llm.Router minimally for the goal hook (only
// ClientFor is used by the judge package).
type stubGoalRouter struct {
	client *stubGoalClient
}

func (r *stubGoalRouter) ClientFor(_ llm.Role) (llm.Client, llm.TierResolution, error) {
	return r.client, llm.TierResolution{}, nil
}
func (r *stubGoalRouter) ClientForTier(_ llm.Tier) (llm.Client, llm.TierResolution, error) {
	return r.client, llm.TierResolution{}, nil
}
func (r *stubGoalRouter) TierForRole(_ llm.Role) llm.Tier { return llm.TierStandard }
func (r *stubGoalRouter) DefaultTier() llm.Tier           { return llm.TierStandard }

type stubGoalClient struct {
	response string
}

func (s *stubGoalClient) Ask(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubGoalClient) Chat(_ context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: s.response}}, nil
}

func newGoalHookFixture(t *testing.T, judgeResp string) (chatHandlerDeps, chatRunState, *chatStreamWriter, *session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	_, err = store.SetGoal(main.ID, &session.SessionGoal{Description: "win", MaxAutoContinues: 2})
	if err != nil {
		t.Fatalf("set goal: %v", err)
	}
	fresh, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	deps := chatHandlerDeps{
		logger: zerolog.New(io.Discard),
		router: &stubGoalRouter{client: &stubGoalClient{response: judgeResp}},
	}
	state := chatRunState{
		store:       store,
		sessionID:   main.ID,
		sessionGoal: fresh.Goal,
		llmMessages: []llm.ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "do it"},
		},
	}
	rec := httptest.NewRecorder()
	stream := newChatStreamWriter(rec, main.ID, deps.logger)
	return deps, state, stream, store, main.ID
}

func TestGoalHook_NilWhenNoActiveGoal(t *testing.T) {
	deps := chatHandlerDeps{logger: zerolog.New(io.Discard)}
	state := chatRunState{} // no goal
	if hook := buildGoalAwareTurnEndHook(deps, state, nil); hook != nil {
		t.Fatal("expected nil hook when goal is absent")
	}
}

func TestGoalHook_SatisfiedClearsGoal(t *testing.T) {
	deps, state, stream, store, sessionID := newGoalHookFixture(t, `{"satisfied": true, "reason": "ok"}`)
	hook := buildGoalAwareTurnEndHook(deps, state, stream)
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}
	input, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "done"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if input != "" {
		t.Fatalf("expected empty input (stop), got %q", input)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal != nil {
		t.Fatalf("expected goal cleared, got %+v", sess.Goal)
	}
}

func TestGoalHook_NotSatisfiedAutoContinues(t *testing.T) {
	deps, state, stream, store, sessionID := newGoalHookFixture(t, `{"satisfied": false, "reason": "wip"}`)
	hook := buildGoalAwareTurnEndHook(deps, state, stream)
	input, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "step 1 done"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.Contains(input, "auto-continue") {
		t.Fatalf("expected auto-continue payload, got %q", input)
	}
	// The agent is told why the judge is not satisfied, and that finished
	// work wants a report rather than a repeat.
	if !strings.Contains(input, "not satisfied yet: wip") || !strings.Contains(input, "do not repeat it") {
		t.Fatalf("expected the judge's reason and the report hint, got %q", input)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal == nil || sess.Goal.AutoContinueCount != 1 {
		t.Fatalf("count not bumped: %+v", sess.Goal)
	}
	if sess.Goal.Status != session.SessionGoalStatusActive {
		t.Fatalf("expected active after auto-continue, got %q", sess.Goal.Status)
	}
}

func TestGoalHook_ExhaustsWhenAtCap(t *testing.T) {
	deps, state, stream, store, sessionID := newGoalHookFixture(t, `{"satisfied": false, "reason": "wip"}`)
	// Pre-bump the goal to its cap so the next "not satisfied" should
	// exhaust rather than auto-continue.
	if _, err := store.UpdateGoalProgress(sessionID, func(g *session.SessionGoal) *session.SessionGoal {
		g.AutoContinueCount = g.MaxAutoContinues
		return g
	}); err != nil {
		t.Fatalf("prep: %v", err)
	}
	// Refresh state.sessionGoal so the hook's pre-flight IsActive check passes.
	fresh, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	state.sessionGoal = fresh.Goal

	hook := buildGoalAwareTurnEndHook(deps, state, stream)
	input, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "still working"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if input != "" {
		t.Fatalf("expected stop after exhaust, got %q", input)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal == nil || sess.Goal.Status != session.SessionGoalStatusExhausted {
		t.Fatalf("expected exhausted, got %+v", sess.Goal)
	}
}

func TestGoalHook_JudgeErrorFailsOpen(t *testing.T) {
	deps, state, stream, store, sessionID := newGoalHookFixture(t, `not json`)
	hook := buildGoalAwareTurnEndHook(deps, state, stream)
	input, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "x"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if input != "" {
		t.Fatalf("expected stop on judge error (fail-open), got %q", input)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal == nil || sess.Goal.AutoContinueCount != 0 {
		t.Fatalf("expected no progress mutation on judge error, got %+v", sess.Goal)
	}
}

func TestGoalHook_RespectsConcurrentClear(t *testing.T) {
	deps, state, stream, store, sessionID := newGoalHookFixture(t, `{"satisfied": false, "reason": "wip"}`)
	// Simulate user calling DELETE /v1/admin/sessions/{id}/goal between
	// the turn start and the OnTurnEnd hook firing.
	if _, err := store.ClearGoal(sessionID); err != nil {
		t.Fatalf("concurrent clear: %v", err)
	}
	hook := buildGoalAwareTurnEndHook(deps, state, stream)
	input, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "x"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if input != "" {
		t.Fatalf("expected stop when goal was cleared mid-turn, got %q", input)
	}
}

func TestAutoContinueMessage(t *testing.T) {
	if got := autoContinueMessage("  "); !strings.Contains(got, "take the next concrete step") || strings.Contains(got, "not satisfied yet") {
		t.Fatalf("without a reason the plain message is used, got %q", got)
	}
	if got := autoContinueMessage("no verification\n results  shown."); !strings.Contains(got, "not satisfied yet: no verification results shown. If the work is done") {
		t.Fatalf("the reason should be on one line, got %q", got)
	}
	long := autoContinueMessage(strings.Repeat("x", autoContinueReasonMaxLen+50))
	if !strings.Contains(long, strings.Repeat("x", autoContinueReasonMaxLen)+"…") || strings.Contains(long, strings.Repeat("x", autoContinueReasonMaxLen+1)) {
		t.Fatalf("a long reason should be cut at %d characters", autoContinueReasonMaxLen)
	}
}

func TestGoalJudgeWindowCarriesToolEvidence(t *testing.T) {
	_, state, _, _, _ := newGoalHookFixture(t, `{"satisfied": true, "reason": "ok"}`)
	reply := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "all tests pass"}}

	if window := goalJudgeWindow(state, reply); len(window) != 2 {
		t.Fatalf("no tool calls: want the request and the reply, got %+v", window)
	}

	records := []ToolCallRecord{}
	for i := 0; i < 8; i++ {
		records = append(records, ToolCallRecord{ToolName: "read_file", ToolArgs: fmt.Sprintf(`{"path":"f%d.go"}`, i), ToolResult: "package x"})
	}
	records = append(records, ToolCallRecord{
		ToolName:    "exec",
		ToolArgs:    `{"command":"make test"}`,
		ToolResult:  "FAIL\n" + strings.Repeat("한", 400),
		ToolIsError: true,
	})
	state.turnToolCalls = &records

	window := goalJudgeWindow(state, reply)
	if len(window) != 3 || window[0].Role != "user" || window[1].Role != "tool" || window[2].Content != "all tests pass" {
		t.Fatalf("want request, tool evidence, reply; got %+v", window)
	}
	evidence := window[1].Content
	for _, want := range []string{"9 (1 failed)", "Last 6", `exec {"command":"make test"} -> FAILED: FAIL 한`, `f3.go`} {
		if !strings.Contains(evidence, want) {
			t.Fatalf("evidence lacks %q:\n%s", want, evidence)
		}
	}
	if strings.Contains(evidence, "f2.go") {
		t.Fatalf("evidence should hold only the last calls:\n%s", evidence)
	}
	if len(evidence) > goal.MaxRecentMessageContentChars || !utf8.ValidString(evidence) {
		t.Fatalf("evidence is %d bytes (cap %d), valid utf8 %v", len(evidence), goal.MaxRecentMessageContentChars, utf8.ValidString(evidence))
	}
}
