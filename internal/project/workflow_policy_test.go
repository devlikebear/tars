package project

import "testing"

func TestWorkflowPolicyKickoffAndBriefStatus(t *testing.T) {
	policy := DefaultWorkflowPolicy

	if !policy.IsKickoffMessage("todo 앱 만드는 프로젝트 시작해줘") {
		t.Fatalf("expected Korean kickoff hint to be recognized")
	}
	if !policy.IsKickoffMessage("start a project to build the dashboard") {
		t.Fatalf("expected English kickoff hint to be recognized")
	}
	if policy.IsKickoffMessage("/project-start") {
		t.Fatalf("expected slash commands to be excluded from kickoff detection")
	}
	if policy.IsKickoffMessage("review the current code") {
		t.Fatalf("expected non-project prompt to be excluded from kickoff detection")
	}
	if !policy.HasActiveBriefStatus("collecting") {
		t.Fatalf("expected collecting brief to stay active")
	}
	if !policy.HasActiveBriefStatus("ready") {
		t.Fatalf("expected ready brief to stay active")
	}
	if policy.HasActiveBriefStatus("finalized") {
		t.Fatalf("expected finalized brief to be inactive")
	}
}

func TestWorkflowPolicyStateNormalizationAndNextAction(t *testing.T) {
	policy := DefaultWorkflowPolicy

	if got := policy.NormalizeProjectStatePhase("REVIEWING"); got != "reviewing" {
		t.Fatalf("expected reviewing phase, got %q", got)
	}
	if got := policy.NormalizeProjectStatePhase("unknown"); got != "planning" {
		t.Fatalf("expected planning fallback, got %q", got)
	}
	if got := policy.NormalizeProjectStateStatus("BLOCKED"); got != "blocked" {
		t.Fatalf("expected blocked status, got %q", got)
	}
	if got := policy.NormalizeProjectStateStatus("unknown"); got != "active" {
		t.Fatalf("expected active fallback, got %q", got)
	}

	brief := Brief{OpenQuestions: []string{"Who owns the first release?"}}
	if got := policy.DefaultProjectNextAction(brief); got != "Who owns the first release?" {
		t.Fatalf("expected open question to become next action, got %q", got)
	}

	narrative := Brief{Kind: "serial"}
	if got := policy.DefaultProjectNextAction(narrative); got == "" || got == "Review project instructions and define the first executable milestone in STATE.md." {
		t.Fatalf("expected narrative brief to use narrative default, got %q", got)
	}
}

func TestWorkflowPolicyDispatchHelpers(t *testing.T) {
	policy := DefaultWorkflowPolicy

	tasks := []BoardTask{
		{ID: "pm-seed-bootstrap", Status: "todo"},
		{ID: "pm-seed-vertical-slice", Status: "todo"},
	}
	filtered := policy.FilterDispatchableTasks("todo", tasks)
	if len(filtered) != len(tasks) {
		t.Fatalf("expected todo tasks to stay dispatchable without seed heuristic, got %+v", filtered)
	}

	board := Board{Tasks: []BoardTask{
		{ID: "task-1", Status: "review"},
		{ID: "task-2", Status: "done"},
	}}
	if !policy.BoardHasStatus(board, "review") {
		t.Fatalf("expected review stage to be detected")
	}
	if policy.AllBoardTasksDone(board) {
		t.Fatalf("expected mixed board to stay incomplete")
	}
	task, ok := policy.FirstTaskByStatus(board, "review")
	if !ok || task.ID != "task-1" {
		t.Fatalf("expected first review task, got ok=%v task=%+v", ok, task)
	}
}

func TestWorkflowPolicyBoardRuntimeState(t *testing.T) {
	policy := DefaultWorkflowPolicy

	board := Board{Tasks: []BoardTask{
		{ID: "task-1", Status: " todo "},
		{ID: "task-2", Status: "doing"},
		{ID: "task-3", Status: "review"},
		{ID: "task-4", Status: "done"},
	}}

	state := policy.ResolveBoardState(board)
	if state.TaskCount != 4 {
		t.Fatalf("expected 4 tasks, got %+v", state)
	}
	if state.TodoCount != 1 || state.InProgressCount != 1 || state.ReviewCount != 1 || state.DoneCount != 1 {
		t.Fatalf("unexpected board state counts: %+v", state)
	}
	if state.IsEmpty() {
		t.Fatalf("expected populated board to not be empty")
	}
	if !state.HasDispatchableTasks() {
		t.Fatalf("expected todo/review board to stay dispatchable")
	}
	if !state.HasInProgressTasks() {
		t.Fatalf("expected doing task to canonicalize to in_progress")
	}
	if state.AllDone() {
		t.Fatalf("expected mixed board to stay incomplete")
	}

	empty := policy.ResolveBoardState(Board{})
	if !empty.IsEmpty() {
		t.Fatalf("expected empty board state, got %+v", empty)
	}
	if !empty.AllDone() {
		t.Fatalf("expected empty board to stay vacuously done")
	}
}

func TestWorkflowPolicyTasksForDispatchStage(t *testing.T) {
	policy := DefaultWorkflowPolicy

	board := Board{Tasks: []BoardTask{
		{ID: "task-1", Status: "todo"},
		{ID: "task-2", Status: "review"},
		{ID: "task-3", Status: "done"},
	}}

	todo := policy.TasksForDispatchStage(board, " TODO ")
	if len(todo) != 1 || todo[0].ID != "task-1" {
		t.Fatalf("expected todo dispatch stage to return task-1, got %+v", todo)
	}

	review := policy.TasksForDispatchStage(board, "review")
	if len(review) != 1 || review[0].ID != "task-2" {
		t.Fatalf("expected review dispatch stage to return task-2, got %+v", review)
	}

	if tasks := policy.TasksForDispatchStage(board, "blocked"); len(tasks) != 0 {
		t.Fatalf("expected unsupported stage to return no tasks, got %+v", tasks)
	}
}

func TestWorkflowPolicyProjectStateDefaultsAndSummary(t *testing.T) {
	policy := DefaultWorkflowPolicy

	defaultState := policy.DefaultProjectState("project-1")
	if defaultState.ProjectID != "project-1" {
		t.Fatalf("expected project id to round-trip, got %+v", defaultState)
	}
	if defaultState.Phase != "planning" || defaultState.Status != "active" {
		t.Fatalf("expected default planning/active state, got %+v", defaultState)
	}

	initial := policy.InitialProjectState(Brief{
		Goal:          "Ship dashboard",
		OpenQuestions: []string{"Who owns the first release?"},
	})
	if initial.Phase != "planning" || initial.Status != "active" {
		t.Fatalf("expected brief init state to start planning/active, got %+v", initial)
	}
	if initial.NextAction != "Who owns the first release?" {
		t.Fatalf("expected brief next action from open question, got %+v", initial)
	}
	if initial.LastRunSummary != "Project initialized from brief." {
		t.Fatalf("expected brief init summary, got %+v", initial)
	}

	status, phase, nextAction := policy.ProjectStateSummary(Project{Status: "active"}, nil)
	if status != "active" || phase != "planning" || nextAction != "" {
		t.Fatalf("expected dashboard fallback summary, got status=%q phase=%q next=%q", status, phase, nextAction)
	}

	status, phase, nextAction = policy.ProjectStateSummary(Project{Status: "archived"}, &ProjectState{
		Status:     "blocked",
		Phase:      "reviewing",
		NextAction: "Review blocker",
	})
	if status != "blocked" || phase != "reviewing" || nextAction != "Review blocker" {
		t.Fatalf("expected state summary to prefer normalized project state, got status=%q phase=%q next=%q", status, phase, nextAction)
	}
}

func TestWorkflowPolicyRuntimeStateAndStatusSync(t *testing.T) {
	policy := DefaultWorkflowPolicy

	runtimeState := policy.ResolveRuntimeState(Project{Status: "archived"}, &ProjectState{
		Status:      "blocked",
		Phase:       "reviewing",
		NextAction:  "Review blocker",
		PhaseNumber: 2,
	})
	if runtimeState.ProjectStatus != "archived" {
		t.Fatalf("expected archived project status, got %+v", runtimeState)
	}
	if runtimeState.Status != "blocked" || runtimeState.Phase != "reviewing" || runtimeState.NextAction != "Review blocker" {
		t.Fatalf("unexpected runtime state: %+v", runtimeState)
	}
	if !runtimeState.IsProjectArchived() {
		t.Fatalf("expected archived runtime state")
	}
	if !runtimeState.PhaseLimitReached(2) {
		t.Fatalf("expected phase limit to be reached")
	}
	if runtimeState.PlanningReady() {
		t.Fatalf("expected reviewing/blocked state to not be planning ready")
	}

	planningState := policy.ResolveRuntimeState(Project{Status: "active"}, &ProjectState{
		Status:      "active",
		Phase:       "planning",
		PhaseNumber: 1,
	})
	if !planningState.PlanningReady() {
		t.Fatalf("expected planning/active state to be planning ready")
	}
	if planningState.PhaseLimitReached(0) {
		t.Fatalf("expected zero max phases to disable phase limit checks")
	}

	update, ok := policy.StateUpdateForProjectStatus("archived")
	if !ok {
		t.Fatal("expected archived project status sync update")
	}
	if update.Phase != "done" || update.Status != "done" || update.StopReason != "Project archived" {
		t.Fatalf("unexpected archived sync update: %+v", update)
	}

	update, ok = policy.StateUpdateForProjectStatus("active")
	if !ok {
		t.Fatal("expected active project status sync update")
	}
	if update.Phase != "planning" || update.Status != "active" {
		t.Fatalf("unexpected active sync update: %+v", update)
	}

	if _, ok := policy.StateUpdateForProjectStatus("paused"); ok {
		t.Fatal("expected paused status to leave project state unchanged")
	}
}

func TestWorkflowPolicyResolveEvent(t *testing.T) {
	policy := DefaultWorkflowPolicy

	kickoff := policy.ResolveEvent(WorkflowEventKickoffRequested, WorkflowEventContext{
		UserMessage: "start a project to build the dashboard",
	})
	if !kickoff.CreateDedicatedSession {
		t.Fatalf("expected kickoff event to request a dedicated session")
	}

	projectChat := policy.ResolveEvent(WorkflowEventKickoffRequested, WorkflowEventContext{
		UserMessage: "hello",
		ProjectID:   "project-1",
	})
	if !projectChat.CreateDedicatedSession {
		t.Fatalf("expected project chat to always use a dedicated session")
	}

	normalChat := policy.ResolveEvent(WorkflowEventKickoffRequested, WorkflowEventContext{
		UserMessage: "hello",
	})
	if normalChat.CreateDedicatedSession {
		t.Fatalf("expected non-kickoff main chat to stay on the main session")
	}

	briefFinalized := policy.ResolveEvent(WorkflowEventBriefFinalized, WorkflowEventContext{
		Brief: Brief{
			Goal:          "Ship the first release",
			OpenQuestions: []string{"Who owns the first release?"},
		},
	})
	if !briefFinalized.PlanningReady {
		t.Fatalf("expected finalized brief to be planning ready")
	}
	stateInput := briefFinalized.Transition.StateInput()
	if stateInput.Goal == nil || *stateInput.Goal != "Ship the first release" {
		t.Fatalf("expected brief goal to carry into state input, got %+v", stateInput)
	}
	if stateInput.Phase == nil || *stateInput.Phase != "planning" || stateInput.Status == nil || *stateInput.Status != "active" {
		t.Fatalf("expected planning/active state transition, got %+v", stateInput)
	}
	if got := len(stateInput.RemainingTasks); got != 1 || stateInput.RemainingTasks[0] != "Who owns the first release?" {
		t.Fatalf("expected brief open questions to seed remaining tasks, got %+v", stateInput.RemainingTasks)
	}

	planning := policy.ResolveEvent(WorkflowEventPlanningRequested, WorkflowEventContext{
		Project: Project{Status: "active"},
		State: &ProjectState{
			Status:      "blocked",
			Phase:       "reviewing",
			NextAction:  "Review blocker",
			PhaseNumber: 2,
		},
	})
	if planning.PlanningReady {
		t.Fatalf("expected blocked reviewing state to not be planning ready")
	}
	if planning.RuntimeState.Status != "blocked" || planning.RuntimeState.Phase != "reviewing" || planning.RuntimeState.NextAction != "Review blocker" {
		t.Fatalf("unexpected planning runtime state: %+v", planning.RuntimeState)
	}
}
