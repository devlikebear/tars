package tarsserver

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

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
func (r *stubGoalRouter) DefaultTier() llm.Tier            { return llm.TierStandard }

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
