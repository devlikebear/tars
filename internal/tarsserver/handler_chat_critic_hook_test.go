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

func TestCriticHook_SkipsWhenNoPlanTransition(t *testing.T) {
	deps, state, stream, _, _ := newCriticHookFixture(t, `{"acceptable": true, "feedback": "", "reason": "ok"}`, session.PlanStatusExecuting)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "x"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if decision.Status != criticStatusSkip {
		t.Fatalf("expected skip for executing plan, got %v", decision.Status)
	}
}

func TestCriticHook_PlanProposedSatisfied(t *testing.T) {
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `{"acceptable": true, "feedback": "", "reason": "solid"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "here is the plan"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if decision.Status != criticStatusSatisfied {
		t.Fatalf("expected satisfied, got %v", decision.Status)
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

func TestCriticHook_PlanProposedFeedbackInjects(t *testing.T) {
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `{"acceptable": false, "feedback": "- add tests\n- check edge cases", "reason": "incomplete"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "here is the plan"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if decision.Status != criticStatusFeedback {
		t.Fatalf("expected feedback, got %v", decision.Status)
	}
	if !strings.Contains(decision.Injection, criticInjectedMessagePrefix) {
		t.Fatalf("injection missing prefix: %q", decision.Injection)
	}
	if !strings.Contains(decision.Injection, "add tests") {
		t.Fatalf("injection missing feedback body: %q", decision.Injection)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.CurrentIteration != 1 {
		t.Fatalf("iteration = %d, want 1", sess.Critic.CurrentIteration)
	}
	if sess.Critic.LastFeedback == "" {
		t.Fatal("expected LastFeedback persisted")
	}
}

func TestCriticHook_ExhaustsAtBudget(t *testing.T) {
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `{"acceptable": false, "feedback": "- still bad", "reason": "still bad"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	// Iter 1
	if d, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v1"}}); err != nil || d.Status != criticStatusFeedback {
		t.Fatalf("iter1: %v, status=%v", err, d.Status)
	}
	// Re-read state.sessionCritic so subsequent invocations see the bumped iteration.
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	state.sessionCritic = sess.Critic

	// Iter 2 (= max)
	if d, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v2"}}); err != nil || d.Status != criticStatusFeedback {
		t.Fatalf("iter2: %v, status=%v", err, d.Status)
	}
	sess, err = store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	state.sessionCritic = sess.Critic

	// Iter 3 (over budget) → exhausted, no further review.
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v3"}})
	if err != nil {
		t.Fatalf("iter3: %v", err)
	}
	if decision.Status != criticStatusExhausted {
		t.Fatalf("expected exhausted, got %v", decision.Status)
	}
	sess, err = store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.Status != session.SessionCriticStatusExhausted {
		t.Fatalf("status = %q, want exhausted", sess.Critic.Status)
	}
}

func TestCriticHook_FailsOpenOnReviewError(t *testing.T) {
	// Unparseable reviewer output → review() returns err with not-acceptable
	// verdict. Hook should emit judge_error SSE, skip injection, and not
	// mutate critic state.
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `not valid json`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "plan"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if decision.Status != criticStatusSkip {
		t.Fatalf("expected skip on review error, got %v", decision.Status)
	}
	if decision.Injection != "" {
		t.Fatalf("expected empty injection, got %q", decision.Injection)
	}
	// State should still reflect "reviewing" reset (it was bumped by the
	// trigger detection step before review fired) but no iteration count
	// increment.
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Critic.CurrentIteration != 0 {
		t.Fatalf("expected iteration 0 after review error, got %d", sess.Critic.CurrentIteration)
	}
}

func TestCriticHook_NoReReviewAfterSatisfiedSameSig(t *testing.T) {
	deps, state, stream, store, sessionID := newCriticHookFixture(t, `{"acceptable": true, "feedback": "", "reason": "ok"}`, session.PlanStatusProposed)
	hook := buildCriticAwareTurnEndHook(deps, state, stream)
	if d, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v1"}}); err != nil || d.Status != criticStatusSatisfied {
		t.Fatalf("first pass: %v, status=%v", err, d.Status)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	state.sessionCritic = sess.Critic

	// Same plan signature, second turn — should skip (no re-review).
	decision, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "v2"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if decision.Status != criticStatusSkip {
		t.Fatalf("expected skip on same sig after satisfied, got %v", decision.Status)
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
}

func TestDetectCriticTrigger(t *testing.T) {
	cases := []struct {
		name        string
		plan        *session.Plan
		wantTrigger string
	}{
		{"nil plan", nil, ""},
		{"drafting", &session.Plan{Status: session.PlanStatusDrafting}, ""},
		{"executing", &session.Plan{Status: session.PlanStatusExecuting}, ""},
		{"proposed", &session.Plan{Status: session.PlanStatusProposed, UpdatedAt: "2026-01-01"}, "plan_proposed"},
		{"completed", &session.Plan{Status: session.PlanStatusCompleted, UpdatedAt: "2026-01-01"}, "plan_completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trigger, sig := detectCriticTrigger(tc.plan)
			if trigger != tc.wantTrigger {
				t.Fatalf("trigger = %q, want %q", trigger, tc.wantTrigger)
			}
			if tc.wantTrigger != "" && sig == "" {
				t.Fatal("expected non-empty signature")
			}
		})
	}
}

func TestChainedHook_CriticFeedbackPreemptsGoal(t *testing.T) {
	// Configure both critic and goal active; reviewer returns "not acceptable"
	// → expect critic injection, goal judge must not run.
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetGoal(main.ID, &session.SessionGoal{Description: "win", MaxAutoContinues: 3}); err != nil {
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
		router: &stubGoalRouter{client: &stubGoalClient{response: `{"acceptable": false, "feedback": "- nope", "reason": "incomplete"}`}},
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
	if !strings.Contains(out, criticInjectedMessagePrefix) {
		t.Fatalf("expected critic injection to win, got %q", out)
	}
	// Goal must remain untouched (count still 0).
	sess, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal == nil || sess.Goal.AutoContinueCount != 0 {
		t.Fatalf("expected goal undisturbed, got %+v", sess.Goal)
	}
}

func TestChainedHook_CriticSatisfiedThenGoalRuns(t *testing.T) {
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

	// Single LLM stub used for both reviewer and goal judge — both expect
	// JSON {acceptable|satisfied: false}; here we return "true" so reviewer
	// passes through and the goal judge says satisfied so the loop stops.
	deps := chatHandlerDeps{
		logger: zerolog.New(io.Discard),
		router: &stubGoalRouter{client: &stubGoalClient{response: `{"acceptable": true, "satisfied": true, "feedback": "", "reason": "ok"}`}},
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
	out, err := hook(context.Background(), llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "plan"}})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if out != "" {
		t.Fatalf("expected stop after goal satisfied, got %q", out)
	}
	sess, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.Goal != nil {
		t.Fatalf("goal should be cleared on satisfied, got %+v", sess.Goal)
	}
	if sess.Critic.Status != session.SessionCriticStatusSatisfied {
		t.Fatalf("critic status = %q, want satisfied", sess.Critic.Status)
	}
}
