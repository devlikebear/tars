package focuspipeline

import (
	"fmt"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

// SessionTasks turns an approved plan into the session's plan, tasks and an
// approved TaskContract (ADR §9: focus reuses the session plan/contract).
// The contract's verification commands are the plan's verify commands
// followed by its end-to-end ones; its done criteria are the tasks' done
// statements. The commands were approved by the developer at G1.
func SessionTasks(plan Plan, now time.Time) session.SessionTasks {
	stamp := now.UTC().Format(time.RFC3339)
	tasks := make([]session.Task, 0, len(plan.Tasks))
	done := make([]string, 0, len(plan.Tasks))
	for i, t := range plan.Tasks {
		tasks = append(tasks, session.Task{
			ID:          fmt.Sprintf("task-%d", i+1),
			Title:       t.Title,
			Status:      "pending",
			Description: t.Done,
		})
		if t.Done != "" {
			done = append(done, fmt.Sprintf("%s: %s", t.Title, t.Done))
		}
	}
	verify := append(append([]string{}, plan.Verify...), plan.E2E...)
	return session.SessionTasks{
		Plan: &session.Plan{
			Goal:      plan.Goal,
			CreatedAt: stamp,
			UpdatedAt: stamp,
			Status:    session.PlanStatusExecuting,
		},
		Contract: &session.TaskContract{
			Goal:                 plan.Goal,
			DoneCriteria:         done,
			VerificationCommands: verify,
			Status:               session.ContractStatusApproved,
			CreatedAt:            stamp,
			UpdatedAt:            stamp,
		},
		Tasks: tasks,
	}
}
