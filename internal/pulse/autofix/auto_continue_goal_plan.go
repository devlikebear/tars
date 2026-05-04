package autofix

import (
	"context"
	"fmt"
)

const AutoContinueGoalPlanName = "auto_continue_goal_plan"

type GoalPlanAutoContinueResult struct {
	Continued      int      `json:"continued"`
	GoalsCompleted int      `json:"goals_completed"`
	Skipped        int      `json:"skipped"`
	Escalated      int      `json:"escalated"`
	SessionIDs     []string `json:"session_ids,omitempty"`
}

type GoalPlanAutoContinuer interface {
	ContinueGoalPlans(ctx context.Context) (GoalPlanAutoContinueResult, error)
}

// AutoContinueGoalPlan runs one chat turn per session whose plan has just
// completed with auto-continue opted in. The turn asks the LLM to either
// declare the session goal achieved (which terminates the loop) or propose
// the next plan. A hard iteration cap on the Plan keeps the loop bounded.
type AutoContinueGoalPlan struct {
	Continuer GoalPlanAutoContinuer
}

func (a *AutoContinueGoalPlan) Name() string { return AutoContinueGoalPlanName }

func (a *AutoContinueGoalPlan) Run(ctx context.Context) (Result, error) {
	if a == nil || a.Continuer == nil {
		return Result{Name: AutoContinueGoalPlanName}, fmt.Errorf("goal-plan auto-continuer is not configured")
	}
	result, err := a.Continuer.ContinueGoalPlans(ctx)
	if err != nil {
		return Result{Name: AutoContinueGoalPlanName}, err
	}
	return Result{
		Name:    AutoContinueGoalPlanName,
		Summary: autoContinueGoalPlanSummary(result),
		Changed: result.Continued > 0 || result.GoalsCompleted > 0,
		Details: map[string]any{
			"continued":       result.Continued,
			"goals_completed": result.GoalsCompleted,
			"skipped":         result.Skipped,
			"escalated":       result.Escalated,
			"session_ids":     result.SessionIDs,
		},
	}, nil
}

func autoContinueGoalPlanSummary(result GoalPlanAutoContinueResult) string {
	if result.GoalsCompleted > 0 && result.Continued == 0 {
		return "goal sessions reached their target and stopped"
	}
	if result.Continued > 0 {
		return "kicked off the next plan iteration in opted-in goals"
	}
	if result.Escalated > 0 {
		return "goal sessions hit their iteration cap"
	}
	return "no opted-in goal plans were ready to auto-continue"
}
