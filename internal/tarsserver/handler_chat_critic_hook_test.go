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

// withSyncCriticRunner overrides the package-level async runner so test
// bodies can observe critic side effects deterministically. The previous
// runner is restored on cleanup.
func withSyncCriticRunner(t *testing.T) {
	t.Helper()
	prev := criticAsyncRunner
	criticAsyncRunner = func(fn func()) { fn() }
	t.Cleanup(func() { criticAsyncRunner = prev })
}

func newCriticHookFixture(t *testing.T, reviewerResp string, planStatus string) (chatHandlerDeps, chatRunState, *chatStreamWriter, *session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	_, err = store.SetCritic(main.ID, &session.SessionCritic{Enabled: true, MaxIterations: 2})
	if err != nil {
		t.Fatalf("set critic: %v", err)
	}
	if planStatus != "" {
		if err := store.SaveTasks(main.ID, session.SessionTasks{
			Plan: &session.Plan{
				Goal:      "ship feature",
				Status:    planStatus,
				CreatedAt: "2026-05-12T00:00:00Z",
				UpdatedAt: "2026-05-12T00:00:00Z",
			},
		}); err != nil {
			t.Fatalf("save tasks: %v", err)
		}
	}
	fresh, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	deps := chatHandlerDeps{
		logger: zerolog.New(io.Discard),
		router: &stubGoalRouter{client: &stubGoalClient{response: reviewerResp}},
	}
	state := chatRunState{
		store:         store,
		sessionID:     main.ID,
		sessionCritic: fresh.Critic,
		llmMessages: []llm.ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "do it"},
		},
	}
	rec := httptest.NewRecorder()
	stream := newChatStreamWriter(rec, main.ID, deps.logger)
	return deps, state, stream, store, main.ID
}

func TestCriticHook_NilWhenDisabled(t *testing.T) {
	deps := chatHandlerDeps{logger: zerolog.New(io.Discard)}
	state := chatRunState{}
	if hook := buildCriticAwareTurnEndHook(deps, state, nil); hook != nil {
		t.Fatal("expected nil hook when critic is absent")
	}
}

func TestCriticHook_AssistantTurnSatisfiedClearsStatus(t *testing.T) {
	// No plan → assistant_turn trigger fires.
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": true, "feedback": "", "reason": "looks fine"}`, "")
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "answer"}}); err != nil {
		t.Fatalf("hook err: %v", err)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.Status != session.SessionCriticStatusSatisfied {
		t.Fatalf("critic status = %q, want satisfied", sess.Critic.Status)
	}
	if sess.Critic.PendingFeedback != "" {
		t.Fatalf("expected no pending feedback, got %q", sess.Critic.PendingFeedback)
	}
	if sess.Critic.LastReviewedTurnSig == "" {
		t.Fatal("expected last reviewed turn sig to be recorded")
	}
}

func TestCriticHook_AssistantTurnFeedbackQueuesPending(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": false, "feedback": "- missing edge case", "reason": "gaps"}`, "")
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "answer"}}); err != nil {
		t.Fatalf("hook err: %v", err)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.PendingFeedback == "" {
		t.Fatal("expected PendingFeedback to be queued")
	}
	if !strings.Contains(sess.Critic.PendingFeedback, criticInjectedMessagePrefix) {
		t.Fatalf("pending feedback missing prefix: %q", sess.Critic.PendingFeedback)
	}
	if !strings.Contains(sess.Critic.PendingFeedback, "missing edge case") {
		t.Fatalf("pending feedback missing body: %q", sess.Critic.PendingFeedback)
	}
	if sess.Critic.PendingFeedbackTrigger != "assistant_turn" {
		t.Fatalf("pending trigger = %q", sess.Critic.PendingFeedbackTrigger)
	}
	// assistant_turn does not bump CurrentIteration.
	if sess.Critic.CurrentIteration != 0 {
		t.Fatalf("CurrentIteration = %d, want 0", sess.Critic.CurrentIteration)
	}
}

func TestCriticHook_PlanProposedSatisfied(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": true, "feedback": "", "reason": "solid"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "here is the plan"}}); err != nil {
		t.Fatalf("hook err: %v", err)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.Status != session.SessionCriticStatusSatisfied {
		t.Fatalf("critic status = %q, want satisfied", sess.Critic.Status)
	}
	if sess.Critic.LastReviewedPlanSig == "" {
		t.Fatal("expected plan signature persisted")
	}
}

func TestCriticHook_PlanProposedFeedbackQueuesPending(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": false, "feedback": "- add tests\n- check edge cases", "reason": "incomplete"}`,
		session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "plan"}}); err != nil {
		t.Fatalf("hook err: %v", err)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.PendingFeedback == "" {
		t.Fatal("expected PendingFeedback queued")
	}
	if !strings.Contains(sess.Critic.PendingFeedback, "add tests") {
		t.Fatalf("pending feedback missing body: %q", sess.Critic.PendingFeedback)
	}
	if sess.Critic.CurrentIteration != 1 {
		t.Fatalf("iteration = %d, want 1", sess.Critic.CurrentIteration)
	}
	if sess.Critic.PendingFeedbackTrigger != "plan_proposed" {
		t.Fatalf("pending trigger = %q", sess.Critic.PendingFeedbackTrigger)
	}
}

func TestCriticHook_ExhaustsAtBudgetForPlanTrigger(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": false, "feedback": "- still bad", "reason": "still bad"}`,
		session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	// Iter 1 — queues pending
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v1"}}); err != nil {
		t.Fatalf("iter1: %v", err)
	}
	sess, _ := store.Get(sessionID)
	state.sessionCritic = sess.Critic
	// Drain pending so the next call simulates "user replied, critic feedback consumed".
	if _, err := store.TakePendingCriticFeedback(sessionID); err != nil {
		t.Fatalf("drain: %v", err)
	}
	sess, _ = store.Get(sessionID)
	state.sessionCritic = sess.Critic

	// Iter 2 — queues pending (still bad)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v2"}}); err != nil {
		t.Fatalf("iter2: %v", err)
	}
	sess, _ = store.Get(sessionID)
	state.sessionCritic = sess.Critic
	if _, err := store.TakePendingCriticFeedback(sessionID); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	sess, _ = store.Get(sessionID)
	state.sessionCritic = sess.Critic

	// Iter 3 — over budget, should flip to exhausted without queueing.
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v3"}}); err != nil {
		t.Fatalf("iter3: %v", err)
	}
	sess, _ = store.Get(sessionID)
	if sess.Critic.Status != session.SessionCriticStatusExhausted {
		t.Fatalf("status = %q, want exhausted", sess.Critic.Status)
	}
	if sess.Critic.PendingFeedback != "" {
		t.Fatalf("expected no new pending feedback when exhausted, got %q", sess.Critic.PendingFeedback)
	}
}

func TestCriticHook_FailsOpenOnReviewError(t *testing.T) {
	withSyncCriticRunner(t)
	// Unparseable reviewer output → no pending feedback, no status flip past
	// "reviewing".
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `not valid json`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "plan"}}); err != nil {
		t.Fatalf("hook err: %v", err)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Iteration must not be incremented on review error.
	if sess.Critic.CurrentIteration != 0 {
		t.Fatalf("expected iteration 0 after review error, got %d", sess.Critic.CurrentIteration)
	}
	if sess.Critic.PendingFeedback != "" {
		t.Fatalf("expected no pending feedback on error, got %q", sess.Critic.PendingFeedback)
	}
}

func TestCriticHook_NoReReviewAfterSatisfiedSameSig(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": true, "feedback": "", "reason": "ok"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v1"}}); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	sess, _ := store.Get(sessionID)
	state.sessionCritic = sess.Critic

	// Same plan signature, second turn — should NOT call the reviewer again
	// (status stays satisfied, no new pending).
	if err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v2"}}); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	sess, _ = store.Get(sessionID)
	if sess.Critic.Status != session.SessionCriticStatusSatisfied {
		t.Fatalf("status = %q, want still satisfied", sess.Critic.Status)
	}
}

func TestCriticHook_NoReReviewSameAssistantTurnSig(t *testing.T) {
	withSyncCriticRunner(t)
	deps, state, stream, store, sessionID := newCriticHookFixture(t,
		`{"acceptable": false, "feedback": "- gap", "reason": "gap"}`, "")
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	resp := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "same answer"}}
	if err := hook(context.Background(), resp); err != nil {
		t.Fatalf("first: %v", err)
	}
	sess, _ := store.Get(sessionID)
	firstFeedback := sess.Critic.PendingFeedback
	state.sessionCritic = sess.Critic

	// Same response content → same sig → second call must be a no-op.
	if err := hook(context.Background(), resp); err != nil {
		t.Fatalf("second: %v", err)
	}
	sess, _ = store.Get(sessionID)
	if sess.Critic.PendingFeedback != firstFeedback {
		t.Fatalf("pending feedback changed on duplicate sig: %q -> %q",
			firstFeedback, sess.Critic.PendingFeedback)
	}
}

func TestFormatSessionCriticPrompt(t *testing.T) {
	if got := formatSessionCriticPrompt(nil); got != "" {
		t.Fatalf("nil critic → %q, want empty", got)
	}
	if got := formatSessionCriticPrompt(&session.SessionCritic{Enabled: false}); got != "" {
		t.Fatalf("disabled critic → %q, want empty", got)
	}
	out := formatSessionCriticPrompt(&session.SessionCritic{Enabled: true, MaxIterations: 3})
	if !strings.Contains(out, "Critic agent") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "Maximum review rounds per plan transition: 3") {
		t.Fatalf("missing max-rounds line: %q", out)
	}
	if !strings.Contains(out, "every assistant turn") {
		t.Fatalf("missing assistant_turn note: %q", out)
	}
}

func TestDetectCriticTrigger(t *testing.T) {
	cases := []struct {
		name        string
		plan        *session.Plan
		respContent string
		wantTrigger string
	}{
		{"no plan with content", nil, "hello", "assistant_turn"},
		{"no plan empty response", nil, "", "assistant_turn"},
		{"drafting → assistant_turn", &session.Plan{Status: session.PlanStatusDrafting}, "hello", "assistant_turn"},
		{"executing → assistant_turn", &session.Plan{Status: session.PlanStatusExecuting}, "hello", "assistant_turn"},
		{"proposed → plan_proposed", &session.Plan{Status: session.PlanStatusProposed, UpdatedAt: "2026-01-01"}, "hello", "plan_proposed"},
		{"completed → plan_completed", &session.Plan{Status: session.PlanStatusCompleted, UpdatedAt: "2026-01-01"}, "hello", "plan_completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trigger, sig := detectCriticTrigger(tc.plan, llm.ChatResponse{Message: llm.ChatMessage{Content: tc.respContent}})
			if trigger != tc.wantTrigger {
				t.Fatalf("trigger = %q, want %q", trigger, tc.wantTrigger)
			}
			if tc.wantTrigger == "assistant_turn" && tc.respContent == "" && sig != "" {
				t.Fatalf("empty response should produce empty sig, got %q", sig)
			}
			if tc.wantTrigger != "" && tc.respContent != "" && sig == "" {
				t.Fatal("expected non-empty signature")
			}
		})
	}
}

func TestChainedHook_CriticDoesNotBlockGoalAnymore(t *testing.T) {
	// Async critic no longer preempts goal. With critic + goal both active,
	// the chained hook should run the goal judge normally; critic side
	// effects appear in PendingFeedback.
	withSyncCriticRunner(t)
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetGoal(main.ID, &session.SessionGoal{Description: "win", MaxAutoContinues: 2}); err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if _, err := store.SetCritic(main.ID, &session.SessionCritic{Enabled: true, MaxIterations: 2}); err != nil {
		t.Fatalf("set critic: %v", err)
	}
	if err := store.SaveTasks(main.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "x", Status: session.PlanStatusProposed, UpdatedAt: "2026-05-12T00:00:00Z"},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}
	fresh, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	deps := chatHandlerDeps{
		logger: zerolog.New(io.Discard),
		router: &stubGoalRouter{client: &stubGoalClient{response: `{"acceptable": false, "satisfied": true, "feedback": "- nope", "reason": "incomplete"}`}},
	}
	state := chatRunState{
		store:         store,
		sessionID:     main.ID,
		sessionGoal:   fresh.Goal,
		sessionCritic: fresh.Critic,
		llmMessages: []llm.ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "do"},
		},
	}
	rec := httptest.NewRecorder()
	stream := newChatStreamWriter(rec, main.ID, deps.logger)
	hook := buildChatTurnEndHook(deps, state, stream)
	if hook == nil {
		t.Fatal("expected non-nil chained hook")
	}
	out, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "plan"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	// Goal judge said satisfied → chained hook returns "" (no auto-continue).
	if out != "" {
		t.Fatalf("expected empty injection from chained hook, got %q", out)
	}
	sess, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Critic side-channel: pending feedback was queued.
	if sess.Critic.PendingFeedback == "" {
		t.Fatal("expected critic to queue pending feedback")
	}
}
