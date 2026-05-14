package critic

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

type stubRouter struct {
	resp string
	err  error
}

func (r *stubRouter) ClientFor(_ llm.Role) (llm.Client, llm.TierResolution, error) {
	return &stubClient{resp: r.resp, err: r.err}, llm.TierResolution{}, nil
}

type stubClient struct {
	resp string
	err  error
}

func (c *stubClient) Ask(_ context.Context, _ string) (string, error) { return "", nil }
func (c *stubClient) Chat(_ context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: c.resp}}, nil
}

func TestParseVerdict_Acceptable(t *testing.T) {
	v, err := ParseVerdict(`{"acceptable": true, "feedback": "", "reason": "looks good"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !v.Acceptable || v.Reason != "looks good" {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestParseVerdict_TolerantOfCodeFences(t *testing.T) {
	raw := "```json\n{\"acceptable\": false, \"feedback\": \"- missing tests\", \"reason\": \"incomplete\"}\n```"
	v, err := ParseVerdict(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.Acceptable || !strings.Contains(v.Feedback, "missing tests") {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestParseVerdict_RejectsEmpty(t *testing.T) {
	if _, err := ParseVerdict("   "); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseVerdict_RejectsNonJSON(t *testing.T) {
	if _, err := ParseVerdict("plain text without braces"); err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestReview_PlanProposedSatisfied(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `{"acceptable": true, "feedback": "", "reason": "solid"}`}, "")
	plan := &session.Plan{Goal: "ship feature X", Status: session.PlanStatusProposed}
	v, err := r.Review(context.Background(), TriggerPlanProposed, plan, nil, nil)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !v.Acceptable {
		t.Fatalf("expected acceptable, got %+v", v)
	}
}

func TestReview_PlanCompletedFeedback(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `{"acceptable": false, "feedback": "- no verification step", "reason": "needs tests"}`}, "")
	plan := &session.Plan{Goal: "refactor module", Status: session.PlanStatusCompleted}
	tasks := []session.Task{{ID: "1", Title: "do work", Status: "completed"}}
	v, err := r.Review(context.Background(), TriggerPlanCompleted, plan, tasks, nil)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if v.Acceptable {
		t.Fatalf("expected not acceptable, got %+v", v)
	}
	if !strings.Contains(v.Feedback, "no verification") {
		t.Fatalf("feedback missing detail: %q", v.Feedback)
	}
}

func TestReview_UnknownTriggerErrors(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `{"acceptable": true}`}, "")
	plan := &session.Plan{Goal: "x", Status: session.PlanStatusExecuting}
	if _, err := r.Review(context.Background(), "executing", plan, nil, nil); err == nil {
		t.Fatal("expected error for unknown trigger")
	}
}

func TestReview_AssistantTurnAllowsNilPlan(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `{"acceptable": false, "feedback": "- missed", "reason": "gap"}`}, "")
	v, err := r.Review(context.Background(), TriggerAssistantTurn, nil, nil, []llm.ChatMessage{
		{Role: "user", Content: "what's 2+2?"},
		{Role: "assistant", Content: "5"},
	})
	if err != nil {
		t.Fatalf("assistant_turn with nil plan should succeed, got: %v", err)
	}
	if v.Acceptable {
		t.Fatalf("expected acceptable=false, got %+v", v)
	}
	if v.Feedback == "" {
		t.Fatal("expected non-empty feedback")
	}
}

func TestReview_PlanProposedStillRequiresPlan(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `{"acceptable": true}`}, "")
	if _, err := r.Review(context.Background(), TriggerPlanProposed, nil, nil, nil); err == nil {
		t.Fatal("expected error: plan triggers still require a plan")
	}
	if _, err := r.Review(context.Background(), TriggerPlanCompleted, nil, nil, nil); err == nil {
		t.Fatal("expected error: plan_completed still requires a plan")
	}
}

func TestReview_UnparseableReturnsFailOpenError(t *testing.T) {
	r := NewLLMReviewer(&stubRouter{resp: `not json at all`}, "")
	plan := &session.Plan{Goal: "x", Status: session.PlanStatusProposed}
	v, err := r.Review(context.Background(), TriggerPlanProposed, plan, nil, nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
	// The returned verdict should explicitly be not-acceptable so callers
	// that ignore the error and use the verdict do not auto-accept on a
	// reviewer outage.
	if v.Acceptable {
		t.Fatalf("expected acceptable=false on parse failure, got %+v", v)
	}
}
