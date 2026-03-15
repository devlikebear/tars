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

func TestWorkflowPolicyDispatchAndAutopilotHelpers(t *testing.T) {
	policy := DefaultWorkflowPolicy

	tasks := []BoardTask{
		{ID: "pm-seed-bootstrap", Status: "todo"},
		{ID: "pm-seed-vertical-slice", Status: "todo"},
	}
	filtered := policy.FilterDispatchableTasks("todo", tasks)
	if len(filtered) != 1 || filtered[0].ID != "pm-seed-bootstrap" {
		t.Fatalf("expected bootstrap-only dispatch, got %+v", filtered)
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

	if !policy.ShouldAutopilotRun(Project{Status: "active"}, Board{}, nil) {
		t.Fatalf("expected empty board to keep autopilot eligible for seeding")
	}
	if policy.ShouldAutopilotRun(Project{Status: "archived"}, Board{}, nil) {
		t.Fatalf("expected archived project to skip autopilot")
	}
	doneState := &ProjectState{Status: "done"}
	doneBoard := Board{Tasks: []BoardTask{{ID: "task-1", Status: "done"}}}
	if policy.ShouldAutopilotRun(Project{Status: "active"}, doneBoard, doneState) {
		t.Fatalf("expected done project state to stop autopilot")
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

func TestWorkflowPolicyAutopilotStateUpdates(t *testing.T) {
	policy := DefaultWorkflowPolicy

	seeded := policy.AutopilotSeededBacklogState()
	if seeded.Phase != "planning" || seeded.Status != "active" || seeded.NextAction != "Dispatch seeded backlog" {
		t.Fatalf("unexpected seeded backlog state: %+v", seeded)
	}

	todo, ok := policy.AutopilotDispatchState("todo")
	if !ok || todo.Phase != "executing" || todo.Status != "active" || todo.NextAction != "Dispatch todo tasks" {
		t.Fatalf("unexpected todo dispatch state: ok=%v state=%+v", ok, todo)
	}
	review, ok := policy.AutopilotDispatchState("review")
	if !ok || review.Phase != "reviewing" || review.Status != "active" || review.NextAction != "Dispatch review tasks" {
		t.Fatalf("unexpected review dispatch state: ok=%v state=%+v", ok, review)
	}
	if _, ok := policy.AutopilotDispatchState("unknown"); ok {
		t.Fatalf("expected unknown dispatch stage to be rejected")
	}

	recovered := policy.AutopilotRecoveredState("PM auto-requeued stalled task: task-1")
	if recovered.Phase != "executing" || recovered.Status != "active" || recovered.NextAction != "Retry recovered tasks" {
		t.Fatalf("unexpected recovered state: %+v", recovered)
	}

	blocked := policy.AutopilotBlockedState("Autopilot waiting on task: task-1", "")
	if blocked.Phase != "blocked" || blocked.Status != "blocked" || blocked.NextAction != "Autopilot will retry after the loop interval" {
		t.Fatalf("unexpected blocked state: %+v", blocked)
	}

	failed := policy.AutopilotFailedState("runner crashed")
	if failed.Phase != "blocked" || failed.Status != "blocked" || failed.NextAction != "Inspect autopilot failure" || failed.StopReason != "runner crashed" {
		t.Fatalf("unexpected failure state: %+v", failed)
	}

	done := policy.AutopilotCompletedState("Autopilot completed all project tasks")
	if done.Phase != "done" || done.Status != "done" || done.NextAction != "Project complete" || done.CompletionSummary != "Autopilot completed all project tasks" {
		t.Fatalf("unexpected completed state: %+v", done)
	}

	recoveredTasks, recoveredIDs, ok := policy.RecoverStalledTasks([]BoardTask{
		{ID: "task-1", Status: "in_progress"},
		{ID: "task-2", Status: "done"},
	})
	if !ok {
		t.Fatalf("expected stalled task recovery to be detected")
	}
	if len(recoveredIDs) != 1 || recoveredIDs[0] != "task-1" {
		t.Fatalf("unexpected recovered ids: %v", recoveredIDs)
	}
	if recoveredTasks[0].Status != "todo" || recoveredTasks[1].Status != "done" {
		t.Fatalf("unexpected recovered tasks: %+v", recoveredTasks)
	}
}
