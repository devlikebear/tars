package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

// newProjectProgressAfterHeartbeat returns a callback that advances active
// projects after each heartbeat tick. Autonomous projects get full
// plan→execute→review cycle; manual projects only dispatch existing todos.
func newProjectProgressAfterHeartbeat(
	store *project.Store,
	runner project.TaskRunner,
	ask heartbeat.AskFunc,
	logger zerolog.Logger,
) func(ctx context.Context) error {
	if store == nil || runner == nil {
		return nil
	}
	return func(ctx context.Context) error {
		projects, err := store.List()
		if err != nil {
			return err
		}
		for _, p := range projects {
			if strings.TrimSpace(p.Status) == "archived" {
				continue
			}
			if strings.TrimSpace(p.ExecutionMode) == "autonomous" {
				advanceAutonomousProject(ctx, store, runner, ask, p, logger)
			} else {
				advanceManualProject(ctx, store, runner, p, logger)
			}
		}
		return nil
	}
}

// advanceManualProject dispatches existing todo/review tasks only.
func advanceManualProject(ctx context.Context, store *project.Store, runner project.TaskRunner, p project.Project, logger zerolog.Logger) {
	board, err := store.GetBoard(p.ID)
	if err != nil {
		return
	}
	dispatchBoardTasks(ctx, store, runner, p.ID, board, logger)
}

// advanceAutonomousProject runs the full autonomous cycle:
// 1. Check phase limit
// 2. If board empty → LLM generates tasks (planning)
// 3. If has todo/review → dispatch
// 4. If all done → advance phase or complete project
func advanceAutonomousProject(ctx context.Context, store *project.Store, runner project.TaskRunner, ask heartbeat.AskFunc, p project.Project, logger zerolog.Logger) {
	state, err := store.GetState(p.ID)
	if err != nil {
		state = project.ProjectState{ProjectID: p.ID}
	}

	// Check phase limit
	maxPhases := p.MaxPhases
	if maxPhases <= 0 {
		maxPhases = 3
	}
	if state.PhaseNumber >= maxPhases {
		if state.Status != "done" {
			completeProject(store, p.ID, "Maximum phases reached", logger)
		}
		return
	}

	board, err := store.GetBoard(p.ID)
	if err != nil {
		return
	}

	// Count task statuses
	counts := countTaskStatuses(board)

	// Case 1: Board empty → plan first phase
	if len(board.Tasks) == 0 {
		if ask == nil {
			logger.Debug().Str("project_id", p.ID).Msg("autonomous: skip planning (no LLM)")
			return
		}
		nextPhase := state.PhaseNumber + 1
		planAutonomousTasks(ctx, store, ask, p, nextPhase, logger)
		return
	}

	// Case 2: All tasks done → critic review (if configured), then next phase
	if counts["done"] == len(board.Tasks) {
		if ask == nil {
			return
		}
		// Run critic review if sub_agents includes "critic"
		if hasCritic(p.SubAgents) {
			runCriticReview(ctx, store, ask, p, board, state.PhaseNumber, logger)
		}
		nextPhase := state.PhaseNumber + 1
		planAutonomousTasks(ctx, store, ask, p, nextPhase, logger)
		return
	}

	// Case 2: Has in_progress tasks → skip (running)
	if counts["in_progress"] > 0 {
		return
	}

	// Case 3: Has todo/review → dispatch
	if counts["todo"] > 0 || counts["review"] > 0 {
		dispatchBoardTasks(ctx, store, runner, p.ID, board, logger)
		return
	}
}

// planAutonomousTasks uses LLM to generate tasks for the next phase.
func planAutonomousTasks(ctx context.Context, store *project.Store, ask heartbeat.AskFunc, p project.Project, phaseNumber int, logger zerolog.Logger) {
	prompt := fmt.Sprintf(
		`You are a project planner. Generate a concise task list for phase %d of this project.

Project: %s
Objective: %s
Instructions: %s

Return a JSON array of task objects. Each task has "id" (string), "title" (string).
Generate 1-5 concrete, actionable tasks. Only return the JSON array, nothing else.
Example: [{"id":"task-1","title":"Write introduction section"}]`,
		phaseNumber,
		strings.TrimSpace(p.Name),
		strings.TrimSpace(p.Objective),
		strings.TrimSpace(p.Body),
	)

	response, err := ask(ctx, prompt)
	if err != nil {
		logger.Debug().Err(err).Str("project_id", p.ID).Msg("autonomous: planning LLM call failed")
		return
	}

	// Parse tasks from LLM response
	tasks := parseTasksFromLLM(response)
	if len(tasks) == 0 {
		logger.Debug().Str("project_id", p.ID).Str("response", response).Msg("autonomous: no tasks parsed from LLM")
		return
	}

	// Update board with new tasks
	boardTasks := make([]project.BoardTask, len(tasks))
	for i, t := range tasks {
		boardTasks[i] = project.BoardTask{
			ID:     fmt.Sprintf("phase%d-task-%d", phaseNumber, i+1),
			Title:  t.Title,
			Status: "todo",
		}
	}
	if _, err := store.UpdateBoard(p.ID, project.BoardUpdateInput{Tasks: boardTasks}); err != nil {
		logger.Debug().Err(err).Str("project_id", p.ID).Msg("autonomous: update board failed")
		return
	}

	// Update state
	phaseName := "executing"
	statusName := "active"
	nextAction := fmt.Sprintf("Phase %d: execute %d tasks", phaseNumber, len(tasks))
	_, _ = store.UpdateState(p.ID, project.ProjectStateUpdateInput{
		Phase:       &phaseName,
		Status:      &statusName,
		PhaseNumber: &phaseNumber,
		NextAction:  &nextAction,
	})

	logger.Info().
		Str("project_id", p.ID).
		Int("phase", phaseNumber).
		Int("tasks", len(tasks)).
		Msg("autonomous: planned new phase")
}

// completeProject marks a project as done.
func completeProject(store *project.Store, projectID string, reason string, logger zerolog.Logger) {
	donePhase := "done"
	doneStatus := "done"
	_, _ = store.UpdateState(projectID, project.ProjectStateUpdateInput{
		Phase:             &donePhase,
		Status:            &doneStatus,
		CompletionSummary: &reason,
	})
	archivedStatus := "archived"
	_, _ = store.Update(projectID, project.UpdateInput{Status: &archivedStatus})
	logger.Info().Str("project_id", projectID).Str("reason", reason).Msg("autonomous: project completed")
}

// dispatchBoardTasks dispatches todo and review tasks via orchestrator.
func dispatchBoardTasks(ctx context.Context, store *project.Store, runner project.TaskRunner, projectID string, board project.Board, logger zerolog.Logger) {
	hasTodo := false
	hasReview := false
	for _, task := range board.Tasks {
		switch strings.TrimSpace(task.Status) {
		case "todo":
			hasTodo = true
		case "review":
			hasReview = true
		}
	}
	if !hasTodo && !hasReview {
		return
	}
	orch := project.NewOrchestrator(store, runner)
	if hasTodo {
		report, err := orch.DispatchTodo(ctx, projectID)
		if err != nil {
			logger.Debug().Err(err).Str("project_id", projectID).Msg("project progress: dispatch todo failed")
		} else if len(report.Runs) > 0 {
			logger.Info().Str("project_id", projectID).Int("dispatched", len(report.Runs)).Msg("project progress: dispatched todo tasks")
		}
	}
	if hasReview {
		report, err := orch.DispatchReview(ctx, projectID)
		if err != nil {
			logger.Debug().Err(err).Str("project_id", projectID).Msg("project progress: dispatch review failed")
		} else if len(report.Runs) > 0 {
			logger.Info().Str("project_id", projectID).Int("dispatched", len(report.Runs)).Msg("project progress: dispatched review tasks")
		}
	}
}

func countTaskStatuses(board project.Board) map[string]int {
	counts := map[string]int{}
	for _, t := range board.Tasks {
		counts[strings.TrimSpace(t.Status)]++
	}
	return counts
}

type llmTask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func parseTasksFromLLM(response string) []llmTask {
	response = strings.TrimSpace(response)
	// Try to extract JSON array from response
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start >= 0 && end > start {
		response = response[start : end+1]
	}
	var tasks []llmTask
	if err := json.Unmarshal([]byte(response), &tasks); err != nil {
		return nil
	}
	// Filter out empty titles
	var result []llmTask
	for _, t := range tasks {
		if strings.TrimSpace(t.Title) != "" {
			result = append(result, t)
		}
	}
	return result
}

func hasCritic(subAgents []string) bool {
	for _, a := range subAgents {
		if strings.EqualFold(strings.TrimSpace(a), "critic") {
			return true
		}
	}
	return false
}

// runCriticReview asks the LLM to critically review the completed tasks
// and logs the feedback to project activity.
func runCriticReview(ctx context.Context, store *project.Store, ask heartbeat.AskFunc, p project.Project, board project.Board, phaseNumber int, logger zerolog.Logger) {
	taskSummary := ""
	for _, t := range board.Tasks {
		taskSummary += fmt.Sprintf("- [%s] %s\n", t.Status, t.Title)
	}

	prompt := fmt.Sprintf(
		`You are a critical reviewer. Review the following completed work for project "%s".

Objective: %s

Completed tasks (phase %d):
%s

Provide a brief critical review (3-5 sentences):
1. What was done well?
2. What is missing or could be improved?
3. Should the next phase address any gaps?

Be constructive but honest. Reply in plain text only.`,
		p.Name, p.Objective, phaseNumber, taskSummary,
	)

	response, err := ask(ctx, prompt)
	if err != nil {
		logger.Debug().Err(err).Str("project_id", p.ID).Msg("autonomous: critic review failed")
		return
	}

	// Record review as activity
	_, _ = store.AppendActivity(p.ID, project.ActivityAppendInput{
		Source:  "critic",
		Kind:    "review",
		Status:  "completed",
		Message: strings.TrimSpace(response),
	})
	logger.Info().Str("project_id", p.ID).Int("phase", phaseNumber).Msg("autonomous: critic review completed")
}
