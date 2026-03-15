package project

import "strings"

type WorkflowPolicy struct{}

var DefaultWorkflowPolicy WorkflowPolicy

type workflowStateUpdate struct {
	Phase             string
	Status            string
	NextAction        string
	LastRunSummary    string
	StopReason        string
	CompletionSummary string
}

func (u workflowStateUpdate) stateInput() ProjectStateUpdateInput {
	input := ProjectStateUpdateInput{}
	if trimmed := strings.TrimSpace(u.Phase); trimmed != "" {
		input.Phase = stringValuePtr(trimmed)
	}
	if trimmed := strings.TrimSpace(u.Status); trimmed != "" {
		input.Status = stringValuePtr(trimmed)
	}
	if trimmed := strings.TrimSpace(u.NextAction); trimmed != "" {
		input.NextAction = stringValuePtr(trimmed)
	}
	if trimmed := strings.TrimSpace(u.LastRunSummary); trimmed != "" {
		input.LastRunSummary = stringValuePtr(trimmed)
	}
	if trimmed := strings.TrimSpace(u.StopReason); trimmed != "" {
		input.StopReason = stringValuePtr(trimmed)
	}
	if trimmed := strings.TrimSpace(u.CompletionSummary); trimmed != "" {
		input.CompletionSummary = stringValuePtr(trimmed)
	}
	return input
}

func (WorkflowPolicy) NormalizeBriefStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "collecting", "ready", "finalized":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "collecting"
	}
}

func (p WorkflowPolicy) HasActiveBriefStatus(status string) bool {
	switch p.NormalizeBriefStatus(status) {
	case "collecting", "ready":
		return true
	default:
		return false
	}
}

func (WorkflowPolicy) NormalizeProjectStatePhase(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "planning", "drafting", "executing", "reviewing", "blocked", "done":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "planning"
	}
}

func (WorkflowPolicy) NormalizeProjectStateStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "active", "paused", "blocked", "done":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "active"
	}
}

func (p WorkflowPolicy) DefaultProjectNextAction(brief Brief) string {
	if len(brief.OpenQuestions) > 0 {
		return strings.TrimSpace(brief.OpenQuestions[0])
	}
	if isNarrativeBriefKind(brief.Kind) {
		return "Review STORY_BIBLE.md, CHARACTERS.md, and PLOT.md, then update STATE.md with the first writing milestone."
	}
	return "Review project instructions and define the first executable milestone in STATE.md."
}

func (p WorkflowPolicy) DefaultProjectState(projectID string) ProjectState {
	return ProjectState{
		ProjectID: strings.TrimSpace(projectID),
		Phase:     p.NormalizeProjectStatePhase(""),
		Status:    p.NormalizeProjectStateStatus(""),
	}
}

func (p WorkflowPolicy) InitialProjectState(brief Brief) workflowStateUpdate {
	return workflowStateUpdate{
		Phase:          "planning",
		Status:         "active",
		NextAction:     p.DefaultProjectNextAction(brief),
		LastRunSummary: "Project initialized from brief.",
	}
}

func (p WorkflowPolicy) ProjectStateSummary(item Project, state *ProjectState) (status, phase, nextAction string) {
	status = strings.TrimSpace(item.Status)
	if status == "" {
		status = p.NormalizeProjectStateStatus("")
	}
	phase = p.NormalizeProjectStatePhase("")
	if state == nil {
		return status, phase, ""
	}
	if current := strings.TrimSpace(state.Status); current != "" {
		status = p.NormalizeProjectStateStatus(current)
	}
	if current := strings.TrimSpace(state.Phase); current != "" {
		phase = p.NormalizeProjectStatePhase(current)
	}
	nextAction = strings.TrimSpace(state.NextAction)
	return status, phase, nextAction
}

func (WorkflowPolicy) IsKickoffMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" || strings.HasPrefix(lower, "/") {
		return false
	}
	projectHints := []string{"project", "프로젝트"}
	actionHints := []string{"start", "시작", "만들", "build", "create", "개발", "구축"}
	hasProject := false
	for _, hint := range projectHints {
		if strings.Contains(lower, hint) {
			hasProject = true
			break
		}
	}
	if !hasProject {
		return false
	}
	for _, hint := range actionHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func (WorkflowPolicy) NormalizeDispatchStage(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "todo":
		return "todo", true
	case "review":
		return "review", true
	default:
		return "", false
	}
}

func (WorkflowPolicy) AutopilotSeededBacklogState() workflowStateUpdate {
	return workflowStateUpdate{
		Phase:          "planning",
		Status:         "active",
		NextAction:     "Dispatch seeded backlog",
		LastRunSummary: "PM seeded MVP backlog",
	}
}

func (WorkflowPolicy) AutopilotDispatchState(stage string) (workflowStateUpdate, bool) {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "todo":
		return workflowStateUpdate{
			Phase:          "executing",
			Status:         "active",
			NextAction:     "Dispatch todo tasks",
			LastRunSummary: "Autopilot dispatching todo tasks",
		}, true
	case "review":
		return workflowStateUpdate{
			Phase:          "reviewing",
			Status:         "active",
			NextAction:     "Dispatch review tasks",
			LastRunSummary: "Autopilot dispatching review tasks",
		}, true
	default:
		return workflowStateUpdate{}, false
	}
}

func (WorkflowPolicy) AutopilotRecoveredState(message string) workflowStateUpdate {
	return workflowStateUpdate{
		Phase:          "executing",
		Status:         "active",
		NextAction:     "Retry recovered tasks",
		LastRunSummary: strings.TrimSpace(message),
	}
}

func (WorkflowPolicy) AutopilotBlockedState(message string, nextAction string) workflowStateUpdate {
	trimmedMessage := strings.TrimSpace(message)
	trimmedNext := strings.TrimSpace(nextAction)
	if trimmedNext == "" {
		trimmedNext = "Autopilot will retry after the loop interval"
	}
	return workflowStateUpdate{
		Phase:          "blocked",
		Status:         "blocked",
		NextAction:     trimmedNext,
		LastRunSummary: trimmedMessage,
		StopReason:     trimmedMessage,
	}
}

func (WorkflowPolicy) AutopilotFailedState(message string) workflowStateUpdate {
	trimmed := strings.TrimSpace(message)
	return workflowStateUpdate{
		Phase:          "blocked",
		Status:         "blocked",
		NextAction:     "Inspect autopilot failure",
		LastRunSummary: trimmed,
		StopReason:     trimmed,
	}
}

func (WorkflowPolicy) AutopilotCompletedState(message string) workflowStateUpdate {
	trimmed := strings.TrimSpace(message)
	return workflowStateUpdate{
		Phase:             "done",
		Status:            "done",
		NextAction:        "Project complete",
		LastRunSummary:    trimmed,
		CompletionSummary: trimmed,
	}
}

func (WorkflowPolicy) RecoverStalledTasks(tasks []BoardTask) ([]BoardTask, []string, bool) {
	recovered := make([]BoardTask, 0, len(tasks))
	recoveredIDs := make([]string, 0)
	for _, task := range tasks {
		if strings.TrimSpace(task.Status) == "in_progress" {
			task.Status = "todo"
			recoveredIDs = append(recoveredIDs, task.ID)
		}
		recovered = append(recovered, task)
	}
	if len(recoveredIDs) == 0 {
		return nil, nil, false
	}
	return recovered, recoveredIDs, true
}

func (WorkflowPolicy) FilterDispatchableTasks(status string, tasks []BoardTask) []BoardTask {
	if strings.TrimSpace(status) != "todo" || len(tasks) <= 1 {
		return tasks
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == "pm-seed-bootstrap" {
			return []BoardTask{task}
		}
	}
	return tasks
}

func (WorkflowPolicy) BoardHasStatus(board Board, status string) bool {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return true
		}
	}
	return false
}

func (WorkflowPolicy) FirstTaskByStatus(board Board, status string) (BoardTask, bool) {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return task, true
		}
	}
	return BoardTask{}, false
}

func (WorkflowPolicy) AllBoardTasksDone(board Board) bool {
	if len(board.Tasks) == 0 {
		return true
	}
	for _, task := range board.Tasks {
		if task.Status != "done" {
			return false
		}
	}
	return true
}

func (p WorkflowPolicy) ShouldAutopilotRun(item Project, board Board, state *ProjectState) bool {
	if strings.TrimSpace(item.Status) == "archived" {
		return false
	}
	if len(board.Tasks) == 0 {
		return true
	}
	if !p.AllBoardTasksDone(board) {
		return true
	}
	if state == nil {
		return true
	}
	return p.NormalizeProjectStateStatus(state.Status) != "done"
}
