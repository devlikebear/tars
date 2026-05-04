package autofix

import (
	"context"
	"errors"
	"testing"
)

type fakeGoalPlanContinuer struct {
	result GoalPlanAutoContinueResult
	err    error
	calls  int
}

func (f *fakeGoalPlanContinuer) ContinueGoalPlans(ctx context.Context) (GoalPlanAutoContinueResult, error) {
	f.calls++
	return f.result, f.err
}

func TestAutoContinueGoalPlan_ContinuedSetsChanged(t *testing.T) {
	continuer := &fakeGoalPlanContinuer{result: GoalPlanAutoContinueResult{Continued: 1, SessionIDs: []string{"sess"}}}
	fixer := &AutoContinueGoalPlan{Continuer: continuer}

	got, err := fixer.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !got.Changed {
		t.Fatalf("expected changed=true, got %+v", got)
	}
	if got.Name != AutoContinueGoalPlanName {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Details["continued"] != 1 {
		t.Fatalf("details = %+v", got.Details)
	}
}

func TestAutoContinueGoalPlan_GoalCompletedSetsChanged(t *testing.T) {
	continuer := &fakeGoalPlanContinuer{result: GoalPlanAutoContinueResult{GoalsCompleted: 1, SessionIDs: []string{"sess"}}}
	fixer := &AutoContinueGoalPlan{Continuer: continuer}

	got, err := fixer.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !got.Changed {
		t.Fatalf("expected changed=true when GoalsCompleted>0, got %+v", got)
	}
}

func TestAutoContinueGoalPlan_NotConfigured(t *testing.T) {
	fixer := &AutoContinueGoalPlan{}
	if _, err := fixer.Run(context.Background()); err == nil {
		t.Fatalf("expected error when continuer is nil")
	}
}

func TestAutoContinueGoalPlan_PropagatesContinuerError(t *testing.T) {
	continuer := &fakeGoalPlanContinuer{err: errors.New("boom")}
	fixer := &AutoContinueGoalPlan{Continuer: continuer}
	if _, err := fixer.Run(context.Background()); err == nil {
		t.Fatalf("expected error to propagate")
	}
}
