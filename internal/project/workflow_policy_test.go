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

