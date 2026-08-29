package apptool

import (
	"strings"
	"testing"
)

// The tasks tool's read and removal actions had no tests. They are chat-facing
// — the model calls them mid-conversation — so an unexercised error path here
// surfaces as a confusing tool result rather than a caught bug.

func TestTasks_PlanGetReportsNoPlanBeforeOneIsSet(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)

	res := runTasksAction(t, tool, `{"action":"plan_get"}`)
	if res.IsError {
		t.Fatalf("plan_get on an empty session should not be an error: %+v", res)
	}
	decoded := decodeTasksResult(t, res)
	if decoded["message"] != "no plan set for this session" {
		t.Fatalf("expected the no-plan message, got %v", decoded)
	}
}

func TestTasks_PlanGetReturnsPlanContractAndSummary(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"ship the thing"}`)
	runTasksAction(t, tool, `{"action":"add","title":"first step"}`)

	decoded := decodeTasksResult(t, runTasksAction(t, tool, `{"action":"plan_get"}`))
	plan, ok := decoded["plan"].(map[string]any)
	if !ok {
		t.Fatalf("plan missing from plan_get: %v", decoded)
	}
	if plan["goal"] != "ship the thing" {
		t.Errorf("goal = %v, want the value plan_set stored", plan["goal"])
	}
	// The summary is what the chat UI renders, so it has to come back even
	// when the caller only asked for the plan.
	if _, ok := decoded["summary"]; !ok {
		t.Errorf("plan_get returned no summary: %v", decoded)
	}
}

func TestTasks_ListReturnsTasksAndSummary(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"g"}`)
	runTasksAction(t, tool, `{"action":"add","title":"alpha"}`)
	runTasksAction(t, tool, `{"action":"add","title":"beta"}`)

	decoded := decodeTasksResult(t, runTasksAction(t, tool, `{"action":"list"}`))
	tasks, ok := decoded["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks missing from list: %v", decoded)
	}
	if len(tasks) != 2 {
		t.Fatalf("list returned %d tasks, want 2", len(tasks))
	}
	if _, ok := decoded["summary"]; !ok {
		t.Errorf("list returned no summary: %v", decoded)
	}
}

func TestTasks_ListOnAnEmptySessionIsNotAnError(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)

	res := runTasksAction(t, tool, `{"action":"list"}`)
	if res.IsError {
		t.Fatalf("list on an empty session should succeed: %+v", res)
	}
}

func TestTasks_RemoveDropsOnlyTheNamedTask(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"g"}`)
	runTasksAction(t, tool, `{"action":"add","title":"keep me"}`)
	added := decodeTasksResult(t, runTasksAction(t, tool, `{"action":"add","title":"remove me"}`))

	task, ok := added["task"].(map[string]any)
	if !ok {
		t.Fatalf("add did not return the created task: %v", added)
	}
	id, _ := task["id"].(string)
	if id == "" {
		t.Fatalf("created task has no id: %v", task)
	}

	runTasksAction(t, tool, `{"action":"remove","id":"`+id+`"}`)

	decoded := decodeTasksResult(t, runTasksAction(t, tool, `{"action":"list"}`))
	tasks, _ := decoded["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("after removing one of two tasks, %d remain", len(tasks))
	}
	remaining, _ := tasks[0].(map[string]any)
	if remaining["title"] != "keep me" {
		t.Fatalf("the wrong task was removed; %v remains", remaining["title"])
	}
}

func TestTasks_RemoveRejectsAMissingID(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)

	res := runTasksAction(t, tool, `{"action":"remove"}`)
	if !res.IsError {
		t.Fatalf("remove with no id should be an error result: %+v", res)
	}
	if !strings.Contains(res.Content[0].Text, "id is required") {
		t.Errorf("error does not say what is missing: %q", res.Content[0].Text)
	}
}

func TestTasks_RemoveRejectsAnUnknownID(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"g"}`)
	runTasksAction(t, tool, `{"action":"add","title":"only one"}`)

	res := runTasksAction(t, tool, `{"action":"remove","id":"task-does-not-exist"}`)
	if !res.IsError {
		t.Fatalf("remove with an unknown id should be an error result: %+v", res)
	}
	// The id has to appear in the message, or the model cannot tell which of
	// several removals failed.
	if !strings.Contains(res.Content[0].Text, "task-does-not-exist") {
		t.Errorf("error does not name the missing id: %q", res.Content[0].Text)
	}
}

func TestTasks_ClearEmptiesPlanAndTasks(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"g"}`)
	runTasksAction(t, tool, `{"action":"add","title":"alpha"}`)

	if res := runTasksAction(t, tool, `{"action":"clear"}`); res.IsError {
		t.Fatalf("clear returned an error: %+v", res)
	}

	decoded := decodeTasksResult(t, runTasksAction(t, tool, `{"action":"list"}`))
	if tasks, _ := decoded["tasks"].([]any); len(tasks) != 0 {
		t.Fatalf("clear left %d tasks behind", len(tasks))
	}
	if plan := decoded["plan"]; plan != nil {
		t.Fatalf("clear left a plan behind: %v", plan)
	}
}

func TestTasks_ClearOnAnEmptySessionIsNotAnError(t *testing.T) {
	// Clear archives to memory first, and the archive path has to tolerate
	// there being nothing to archive.
	tool, _, _ := newTasksToolForTest(t)

	if res := runTasksAction(t, tool, `{"action":"clear"}`); res.IsError {
		t.Fatalf("clear on an empty session should succeed: %+v", res)
	}
}
