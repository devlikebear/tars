package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	skillResolver project.SkillResolver,
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
			if project.DefaultWorkflowPolicy.ResolveRuntimeState(p, nil).IsProjectArchived() {
				continue
			}
			if strings.TrimSpace(p.ExecutionMode) == "autonomous" {
				advanceAutonomousProject(ctx, store, runner, ask, skillResolver, p, logger)
			} else {
				advanceManualProject(ctx, store, runner, skillResolver, p, logger)
			}
		}
		return nil
	}
}

// advanceManualProject dispatches existing todo/review tasks only.
func advanceManualProject(ctx context.Context, store *project.Store, runner project.TaskRunner, skillResolver project.SkillResolver, p project.Project, logger zerolog.Logger) {
	board, err := store.GetBoard(p.ID)
	if err != nil {
		return
	}
	dispatchBoardTasks(ctx, store, runner, skillResolver, p.ID, board, logger)
}

// advanceAutonomousProject runs the full autonomous cycle:
// 1. Check phase limit
// 2. If board empty → LLM generates tasks (planning)
// 3. If has todo/review → dispatch
// 4. If all done → advance phase or complete project
func advanceAutonomousProject(ctx context.Context, store *project.Store, runner project.TaskRunner, ask heartbeat.AskFunc, skillResolver project.SkillResolver, p project.Project, logger zerolog.Logger) {
	state, err := store.GetState(p.ID)
	if err != nil {
		state = project.ProjectState{ProjectID: p.ID}
	}
	runtimeState := project.DefaultWorkflowPolicy.ResolveRuntimeState(p, &state)

	// Check phase limit
	maxPhases := p.MaxPhases
	if maxPhases <= 0 {
		maxPhases = 3
	}
	if runtimeState.PhaseLimitReached(maxPhases) {
		if runtimeState.Status != "done" {
			completeProject(store, p.ID, "Maximum phases reached", logger)
		}
		return
	}

	board, err := store.GetBoard(p.ID)
	if err != nil {
		return
	}
	boardState := project.DefaultWorkflowPolicy.ResolveBoardState(board)

	// Case 1: Board empty → plan first phase
	if boardState.IsEmpty() {
		if ask == nil {
			logger.Debug().Str("project_id", p.ID).Msg("autonomous: skip planning (no LLM)")
			return
		}
		nextPhase := runtimeState.PhaseNumber + 1
		planAutonomousTasks(ctx, store, ask, p, nextPhase, "", logger)
		return
	}

	// Case 2: All tasks done → run phase_done agents (if configured), then next phase
	if boardState.AllDone() {
		if ask == nil {
			return
		}
		// Run all sub-agents triggered by "phase_done"
		var agentFeedback string
		phaseDoneAgents := findAgentsByTrigger(p.SubAgents, "phase_done")
		for _, agent := range phaseDoneAgents {
			feedback := runAgentReview(ctx, store, ask, p, board, runtimeState.PhaseNumber, agent, logger)
			if feedback != "" {
				if agentFeedback != "" {
					agentFeedback += "\n\n"
				}
				agentFeedback += fmt.Sprintf("[%s] %s", agent.Role, feedback)
			}
		}
		nextPhase := runtimeState.PhaseNumber + 1
		planAutonomousTasks(ctx, store, ask, p, nextPhase, agentFeedback, logger)
		return
	}

	// Case 2: Has in_progress tasks → skip (running)
	if boardState.HasInProgressTasks() {
		return
	}

	// Case 3: Has todo/review → dispatch
	if boardState.HasDispatchableTasks() {
		dispatchBoardTasks(ctx, store, runner, skillResolver, p.ID, board, logger)
		return
	}
}

// planAutonomousTasks uses LLM to generate tasks for the next phase.
func planAutonomousTasks(ctx context.Context, store *project.Store, ask heartbeat.AskFunc, p project.Project, phaseNumber int, criticFeedback string, logger zerolog.Logger) {
	maxPhases := p.MaxPhases
	if maxPhases <= 0 {
		maxPhases = 3
	}

	// Build context of existing files
	existingFiles := listProjectArtifactNames(store, p.ID)
	filesContext := ""
	if len(existingFiles) > 0 {
		filesContext = fmt.Sprintf("\nExisting files in project: %s", strings.Join(existingFiles, ", "))
	}
	criticContext := ""
	if strings.TrimSpace(criticFeedback) != "" {
		criticContext = fmt.Sprintf("\n\nCRITIC FEEDBACK FROM PREVIOUS PHASE (address these issues):\n%s", strings.TrimSpace(criticFeedback))
	}

	// Build agent context
	agentContext := ""
	if len(p.SubAgents) > 0 {
		var agentNames []string
		for _, a := range p.SubAgents {
			desc := a.Role
			if a.Description != "" {
				desc += " (" + a.Description + ")"
			}
			agentNames = append(agentNames, desc)
		}
		agentContext = fmt.Sprintf("\nAvailable agents: %s", strings.Join(agentNames, ", "))
	}

	prompt := fmt.Sprintf(
		`You are a project executor. Generate tasks for phase %d/%d of this project.

Project: %s
Objective: %s
Instructions: %s
%s%s%s
CRITICAL RULES:
- Each task MUST produce a concrete deliverable file (not a plan, not a template, not analysis).
- If the objective is to write stories → tasks should be "Write story X and save as story-X.md"
- If the objective is to build code → tasks should be "Implement feature X in file Y"
- Do NOT generate planning/analysis/template tasks. The output must be the FINAL deliverable.
- Phase %d of %d: %s

Return a JSON array of task objects. Each task has "id" (string), "title" (string), and optionally "assignee" (string, agent role name).
Generate 1-3 tasks that produce actual deliverables. Only return the JSON array.
Example: [{"id":"task-1","title":"Write the complete first short story and save as story-1.md","assignee":"writer"}]`,
		phaseNumber, maxPhases,
		strings.TrimSpace(p.Name),
		strings.TrimSpace(p.Objective),
		strings.TrimSpace(p.Body),
		filesContext, criticContext, agentContext,
		phaseNumber, maxPhases,
		phaseGuidance(phaseNumber, maxPhases),
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
			ID:       fmt.Sprintf("phase%d-task-%d", phaseNumber, i+1),
			Title:    t.Title,
			Status:   "todo",
			Assignee: strings.TrimSpace(t.Assignee),
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
func dispatchBoardTasks(ctx context.Context, store *project.Store, runner project.TaskRunner, skillResolver project.SkillResolver, projectID string, board project.Board, logger zerolog.Logger) {
	boardState := project.DefaultWorkflowPolicy.ResolveBoardState(board)
	hasTodo := boardState.TodoCount > 0
	hasReview := boardState.ReviewCount > 0
	if !hasTodo && !hasReview {
		return
	}
	orch := project.NewOrchestrator(store, runner)
	orch.SetSkillResolver(skillResolver)
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

type llmTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Assignee string `json:"assignee,omitempty"`
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

func findAgentsByTrigger(agents []project.SubAgentConfig, trigger string) []project.SubAgentConfig {
	var result []project.SubAgentConfig
	for _, a := range agents {
		runAfter := strings.TrimSpace(a.RunAfter)
		if runAfter == "" {
			runAfter = "phase_done"
		}
		if strings.EqualFold(runAfter, trigger) {
			result = append(result, a)
		}
	}
	return result
}

func phaseGuidance(phase, max int) string {
	if phase == 1 {
		return "This is the FIRST phase — create the core deliverables."
	}
	if phase >= max {
		return "This is the FINAL phase — polish, finalize, and complete all remaining deliverables."
	}
	return "Continue building on previous work — add more deliverables or improve existing ones."
}

func listProjectArtifactNames(store *project.Store, projectID string) []string {
	dir := store.ProjectDir(projectID)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	systemFiles := map[string]bool{
		"PROJECT.md": true, "STATE.md": true, "KANBAN.md": true,
		"ACTIVITY.jsonl": true, "AUTOPILOT.json": true,
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || systemFiles[e.Name()] {
			continue
		}
		names = append(names, e.Name())
	}
	return names
}

// runAgentReview asks the LLM to perform a review as the given agent role
// and logs the feedback to project activity.
func runAgentReview(ctx context.Context, store *project.Store, ask heartbeat.AskFunc, p project.Project, board project.Board, phaseNumber int, agent project.SubAgentConfig, logger zerolog.Logger) string {
	taskSummary := ""
	for _, t := range board.Tasks {
		taskSummary += fmt.Sprintf("- [%s] %s\n", t.Status, t.Title)
	}

	roleDesc := agent.Role
	if agent.Description != "" {
		roleDesc = agent.Description
	}

	prompt := fmt.Sprintf(
		`You are a %s. %s

Review the following completed work for project "%s".

Objective: %s

Completed tasks (phase %d):
%s

Provide a brief review (3-5 sentences):
1. What was done well?
2. What is missing or could be improved?
3. Should the next phase address any gaps?

Be constructive but honest. Reply in plain text only.`,
		agent.Role, roleDesc,
		p.Name, p.Objective, phaseNumber, taskSummary,
	)

	response, err := ask(ctx, prompt)
	if err != nil {
		logger.Debug().Err(err).Str("project_id", p.ID).Str("agent", agent.Role).Msg("autonomous: agent review failed")
		return ""
	}

	feedback := strings.TrimSpace(response)

	// Record review as activity
	_, _ = store.AppendActivity(p.ID, project.ActivityAppendInput{
		Source:  agent.Role,
		Kind:    "review",
		Status:  "completed",
		Message: feedback,
	})
	logger.Info().Str("project_id", p.ID).Str("agent", agent.Role).Int("phase", phaseNumber).Msg("autonomous: agent review completed")
	return feedback
}
