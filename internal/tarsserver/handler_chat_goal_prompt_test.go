package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestFormatSessionGoalPrompt_EmptyForInactive(t *testing.T) {
	if got := formatSessionGoalPrompt(nil); got != "" {
		t.Fatalf("expected empty prompt for nil goal, got %q", got)
	}
	inactive := &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusSatisfied}
	if got := formatSessionGoalPrompt(inactive); got != "" {
		t.Fatalf("expected empty prompt for non-active goal, got %q", got)
	}
}

func TestFormatSessionGoalPrompt_ActiveContainsDescriptionWithoutBudget(t *testing.T) {
	goal := &session.SessionGoal{
		Description:       "ship the feature",
		MaxAutoContinues:  5,
		AutoContinueCount: 2,
		Status:            session.SessionGoalStatusActive,
	}
	prompt := formatSessionGoalPrompt(goal)
	if !strings.Contains(prompt, "Active Session Goal") {
		t.Fatalf("missing heading: %q", prompt)
	}
	if !strings.Contains(prompt, "ship the feature") {
		t.Fatalf("missing description: %q", prompt)
	}
	if !strings.Contains(prompt, "do not stop at plan_propose") {
		t.Fatalf("goal must pre-approve plans: %q", prompt)
	}
	// The budget moves on every auto-continue; in the system prompt it would
	// cut the cached prefix each time.
	spent := *goal
	spent.AutoContinueCount = 4
	if formatSessionGoalPrompt(&spent) != prompt {
		t.Fatalf("the goal section must not change as budget is spent: %q", prompt)
	}
}

func TestFormatSessionGoalBudget(t *testing.T) {
	if got := formatSessionGoalBudget(nil); got != "" {
		t.Fatalf("expected no budget line without a goal, got %q", got)
	}
	inactive := &session.SessionGoal{Description: "x", MaxAutoContinues: 2, Status: session.SessionGoalStatusSatisfied}
	if got := formatSessionGoalBudget(inactive); got != "" {
		t.Fatalf("expected no budget line for a finished goal, got %q", got)
	}
	goal := &session.SessionGoal{Description: "x", MaxAutoContinues: 5, AutoContinueCount: 2, Status: session.SessionGoalStatusActive}
	if got := formatSessionGoalBudget(goal); !strings.Contains(got, "3/5") {
		t.Fatalf("missing remaining budget 3/5: %q", got)
	}
	over := &session.SessionGoal{Description: "x", MaxAutoContinues: 2, AutoContinueCount: 9, Status: session.SessionGoalStatusActive}
	if got := formatSessionGoalBudget(over); !strings.Contains(got, "0/2") {
		t.Fatalf("expected budget to clamp to 0/2, got %q", got)
	}
}
