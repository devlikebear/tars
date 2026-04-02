package project

import "strings"

type WorkflowPolicy struct{}

var DefaultWorkflowPolicy WorkflowPolicy

type WorkflowRuntimeState struct {
	ProjectStatus string
	Status        string
	Phase         string
	NextAction    string
	PhaseNumber   int
}

type WorkflowBoardState struct {
	TaskCount       int
	TodoCount       int
	InProgressCount int
	ReviewCount     int
	DoneCount       int
}

type WorkflowEvent string

const (
	WorkflowEventKickoffRequested  WorkflowEvent = "kickoff_requested"
	WorkflowEventBriefFinalized    WorkflowEvent = "brief_finalized"
	WorkflowEventPlanningRequested WorkflowEvent = "planning_requested"
)

type WorkflowEventContext struct {
	UserMessage string
	ProjectID   string
	Project     Project
	State       *ProjectState
	Brief       Brief
}

type WorkflowStateTransition struct {
	Update         workflowStateUpdate
	Goal           string
	RemainingTasks []string
}

type WorkflowEventResolution struct {
	RuntimeState           WorkflowRuntimeState
	Transition             WorkflowStateTransition
	CreateDedicatedSession bool
	PlanningReady          bool
}

func (s WorkflowRuntimeState) PlanningReady() bool {
	return s.Status == "active" && s.Phase == "planning"
}

func (s WorkflowRuntimeState) IsProjectArchived() bool {
	return s.ProjectStatus == "archived"
}

func (s WorkflowRuntimeState) PhaseLimitReached(maxPhases int) bool {
	if maxPhases <= 0 {
		return false
	}
	return s.PhaseNumber >= maxPhases
}

func (t WorkflowStateTransition) StateInput() ProjectStateUpdateInput {
	input := t.Update.stateInput()
	if trimmed := strings.TrimSpace(t.Goal); trimmed != "" {
		input.Goal = stringValuePtr(trimmed)
	}
	if t.RemainingTasks != nil {
		input.RemainingTasks = append([]string(nil), t.RemainingTasks...)
	}
	return input
}

func (s WorkflowBoardState) IsEmpty() bool {
	return s.TaskCount == 0
}

func (s WorkflowBoardState) AllDone() bool {
	return s.TaskCount == 0 || s.DoneCount == s.TaskCount
}

func (s WorkflowBoardState) HasInProgressTasks() bool {
	return s.InProgressCount > 0
}

func (s WorkflowBoardState) HasDispatchableTasks() bool {
	return s.TodoCount > 0 || s.ReviewCount > 0
}

func (s WorkflowBoardState) CountForStatus(status string) int {
	switch canonicalBoardStatus(status) {
	case "todo":
		return s.TodoCount
	case "in_progress":
		return s.InProgressCount
	case "review":
		return s.ReviewCount
	case "done":
		return s.DoneCount
	default:
		return 0
	}
}

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
	runtimeState := p.ResolveRuntimeState(item, state)
	return runtimeState.Status, runtimeState.Phase, runtimeState.NextAction
}

func (p WorkflowPolicy) ResolveRuntimeState(item Project, state *ProjectState) WorkflowRuntimeState {
	runtimeState := WorkflowRuntimeState{
		ProjectStatus: normalizeStatus(item.Status),
		Status:        p.NormalizeProjectStateStatus(""),
		Phase:         p.NormalizeProjectStatePhase(""),
	}
	if state == nil {
		return runtimeState
	}
	if current := strings.TrimSpace(state.Status); current != "" {
		runtimeState.Status = p.NormalizeProjectStateStatus(current)
	}
	if current := strings.TrimSpace(state.Phase); current != "" {
		runtimeState.Phase = p.NormalizeProjectStatePhase(current)
	}
	runtimeState.NextAction = strings.TrimSpace(state.NextAction)
	runtimeState.PhaseNumber = state.PhaseNumber
	return runtimeState
}

func (p WorkflowPolicy) ResolveEvent(event WorkflowEvent, ctx WorkflowEventContext) WorkflowEventResolution {
	switch event {
	case WorkflowEventKickoffRequested:
		return WorkflowEventResolution{
			CreateDedicatedSession: strings.TrimSpace(ctx.ProjectID) != "" || p.IsKickoffMessage(ctx.UserMessage),
		}
	case WorkflowEventBriefFinalized:
		transition := WorkflowStateTransition{
			Update: p.InitialProjectState(ctx.Brief),
			Goal:   strings.TrimSpace(ctx.Brief.Goal),
		}
		if ctx.Brief.OpenQuestions != nil {
			transition.RemainingTasks = append([]string(nil), ctx.Brief.OpenQuestions...)
		}
		runtimeState := p.ResolveRuntimeState(Project{Status: "active"}, &ProjectState{
			Status:     transition.Update.Status,
			Phase:      transition.Update.Phase,
			NextAction: transition.Update.NextAction,
		})
		return WorkflowEventResolution{
			RuntimeState:  runtimeState,
			Transition:    transition,
			PlanningReady: runtimeState.PlanningReady(),
		}
	case WorkflowEventPlanningRequested:
		runtimeState := p.ResolveRuntimeState(ctx.Project, ctx.State)
		return WorkflowEventResolution{
			RuntimeState:  runtimeState,
			PlanningReady: runtimeState.PlanningReady(),
		}
	default:
		return WorkflowEventResolution{}
	}
}

func (p WorkflowPolicy) StateUpdateForProjectStatus(status string) (workflowStateUpdate, bool) {
	switch normalizeStatus(status) {
	case "archived":
		return workflowStateUpdate{
			Phase:      p.NormalizeProjectStatePhase("done"),
			Status:     p.NormalizeProjectStateStatus("done"),
			StopReason: "Project archived",
		}, true
	case "active":
		return workflowStateUpdate{
			Phase:  p.NormalizeProjectStatePhase("planning"),
			Status: p.NormalizeProjectStateStatus("active"),
		}, true
	default:
		return workflowStateUpdate{}, false
	}
}

func (p WorkflowPolicy) StateInputForProjectStatus(status string) (ProjectStateUpdateInput, bool) {
	update, ok := p.StateUpdateForProjectStatus(status)
	if !ok {
		return ProjectStateUpdateInput{}, false
	}
	return update.stateInput(), true
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
	_ = status
	return tasks
}

func (WorkflowPolicy) ResolveBoardState(board Board) WorkflowBoardState {
	state := WorkflowBoardState{}
	for _, task := range board.Tasks {
		state.TaskCount++
		switch canonicalBoardStatus(task.Status) {
		case "todo":
			state.TodoCount++
		case "in_progress":
			state.InProgressCount++
		case "review":
			state.ReviewCount++
		case "done":
			state.DoneCount++
		}
	}
	return state
}

func (p WorkflowPolicy) TasksForDispatchStage(board Board, status string) []BoardTask {
	stage, ok := p.NormalizeDispatchStage(status)
	if !ok {
		return nil
	}
	tasks := make([]BoardTask, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		if canonicalBoardStatus(task.Status) == stage {
			tasks = append(tasks, task)
		}
	}
	return p.FilterDispatchableTasks(stage, tasks)
}

func (p WorkflowPolicy) BoardHasStatus(board Board, status string) bool {
	return p.ResolveBoardState(board).CountForStatus(status) > 0
}

func (WorkflowPolicy) FirstTaskByStatus(board Board, status string) (BoardTask, bool) {
	target := canonicalBoardStatus(status)
	for _, task := range board.Tasks {
		if canonicalBoardStatus(task.Status) == target {
			return task, true
		}
	}
	return BoardTask{}, false
}

func (p WorkflowPolicy) AllBoardTasksDone(board Board) bool {
	return p.ResolveBoardState(board).AllDone()
}
