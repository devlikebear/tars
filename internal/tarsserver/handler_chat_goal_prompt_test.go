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

func TestFormatSessionGoalPrompt_ActiveContainsDescriptionAndBudget(t *testing.T) {
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
	if !strings.Contains(prompt, "3/5") {
		t.Fatalf("missing remaining budget 3/5: %q", prompt)
	}
}

func TestFormatSessionGoalPrompt_NegativeBudgetClampedToZero(t *testing.T) {
	goal := &session.SessionGoal{
		Description:       "x",
		MaxAutoContinues:  2,
		AutoContinueCount: 9, // already over the cap
		Status:            session.SessionGoalStatusActive,
	}
	prompt := formatSessionGoalPrompt(goal)
	if !strings.Contains(prompt, "0/2") {
		t.Fatalf("expected budget to clamp to 0/2, got %q", prompt)
	}
}
