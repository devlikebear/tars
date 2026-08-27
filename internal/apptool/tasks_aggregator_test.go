package apptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

// newTasksToolForTest wires a fresh store + tasks tool against a temp dir.
// Returns the tool and the session ID it operates on so individual tests
// can mutate state through the same Execute path the chat handler uses.
func newTasksToolForTest(t *testing.T) (Tool, string, *session.Store) {
	t.Helper()
	dir := t.TempDir()
	store := session.NewStore(dir)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	tool := NewTasksTool(store, dir, func() string { return main.ID })
	return tool, main.ID, store
}

func runTasksAction(t *testing.T, tool Tool, body string) Result {
	t.Helper()
	res, err := tool.Execute(context.Background(), json.RawMessage(body))
	if err != nil {
		t.Fatalf("execute %s: %v", body, err)
	}
	return res
}

func decodeTasksResult(t *testing.T, res Result) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("expected non-empty result, got %+v", res)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &payload); err != nil {
		t.Fatalf("decode result: %v (raw=%q)", err, res.Content[0].Text)
	}
	return payload
}

func TestTasks_PlanSetSetsDraftingStatus(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"ship feature X"}`)

	st, err := store.GetTasks(sid)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if st.Plan == nil {
		t.Fatal("expected plan to exist after plan_set")
	}
	if st.Plan.Status != session.PlanStatusDrafting {
		t.Errorf("expected status %q after plan_set, got %q", session.PlanStatusDrafting, st.Plan.Status)
	}
	if st.Plan.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set after plan_set")
	}
}

func TestTasks_PlanSetCreatesTaskContractDraft(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{
		"action":"plan_set",
		"goal":"ship feature X",
		"scope":"console only",
		"done_criteria":["tests pass","contract visible"],
		"verification_commands":["make test"],
		"artifacts":["PR"]
	}`)

	st, err := store.GetTasks(sid)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if st.Contract == nil {
		t.Fatal("expected contract draft after plan_set")
	}
	if st.Contract.Status != session.ContractStatusDraft {
		t.Fatalf("expected draft contract, got %+v", st.Contract)
	}
	if st.Contract.Scope != "console only" || len(st.Contract.DoneCriteria) != 2 {
		t.Fatalf("unexpected contract: %+v", st.Contract)
	}
}

func TestTasks_ContractUpdateAndApprove(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"ship feature X"}`)
	runTasksAction(t, tool, `{
		"action":"contract_update",
		"goal":"ship feature X safely",
		"scope":"backend and console",
		"done_criteria":["reload survives"],
		"verification_commands":["go test ./internal/session"],
		"artifacts":["release notes"]
	}`)
	res := runTasksAction(t, tool, `{"action":"contract_approve"}`)
	if res.IsError {
		t.Fatalf("contract_approve unexpectedly errored: %s", res.Content[0].Text)
	}

	st, err := store.GetTasks(sid)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if st.Contract == nil || st.Contract.Status != session.ContractStatusApproved {
		t.Fatalf("expected approved contract, got %+v", st.Contract)
	}
	if st.Contract.Goal != "ship feature X safely" || st.Contract.Artifacts[0] != "release notes" {
		t.Fatalf("unexpected contract after update: %+v", st.Contract)
	}
}

func TestTasks_EvidenceAddAndRemove(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"ship feature X"}`)
	runTasksAction(t, tool, `{"action":"add","title":"Run verification"}`)

	add := runTasksAction(t, tool, `{
		"action":"evidence_add",
		"task_id":"1",
		"type":"test_result",
		"title":"Go tests",
		"summary":"internal/tool passed",
		"command":"go test ./internal/tool",
		"status":"passed"
	}`)
	if add.IsError {
		t.Fatalf("evidence_add unexpectedly errored: %s", add.Content[0].Text)
	}
	st, err := store.GetTasks(sid)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(st.Tasks) != 1 || len(st.Tasks[0].Evidence) != 1 {
		t.Fatalf("expected evidence on task 1, got %+v", st.Tasks)
	}
	ev := st.Tasks[0].Evidence[0]
	if ev.ID == "" || ev.Type != session.EvidenceTypeTestResult || ev.Command != "go test ./internal/tool" {
		t.Fatalf("unexpected evidence: %+v", ev)
	}

	remove := runTasksAction(t, tool, `{"action":"evidence_remove","task_id":"1","evidence_id":"`+ev.ID+`"}`)
	if remove.IsError {
		t.Fatalf("evidence_remove unexpectedly errored: %s", remove.Content[0].Text)
	}
	st, err = store.GetTasks(sid)
	if err != nil {
		t.Fatalf("get tasks after remove: %v", err)
	}
	if len(st.Tasks[0].Evidence) != 0 {
		t.Fatalf("expected evidence removed, got %+v", st.Tasks[0].Evidence)
	}
}

func TestTasks_EvidenceAddRejectsInvalidType(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"ship feature X"}`)
	runTasksAction(t, tool, `{"action":"add","title":"Run verification"}`)

	res := runTasksAction(t, tool, `{
		"action":"evidence_add",
		"task_id":"1",
		"type":"mystery",
		"summary":"unknown"
	}`)
	if !res.IsError {
		t.Fatalf("expected invalid evidence type to error, got %s", res.Content[0].Text)
	}
	if !strings.Contains(res.Content[0].Text, "evidence type") {
		t.Fatalf("expected evidence type error, got %s", res.Content[0].Text)
	}
}

func TestTasks_PlanProposeDraftingToProposed(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	res := runTasksAction(t, tool, `{"action":"plan_propose"}`)

	if res.IsError {
		t.Fatalf("plan_propose unexpectedly errored: %s", res.Content[0].Text)
	}
	st, _ := store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusProposed {
		t.Errorf("expected proposed, got %q", st.Plan.Status)
	}
}

func TestTasks_PlanProposeRejectsExecuting(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	// Set up an executing plan by writing it directly through the store —
	// covers the legacy migration path too (no Status set, defaults to executing).
	now := session.NowRFC3339()
	if err := store.SaveTasks(sid, session.SessionTasks{
		Plan: &session.Plan{Goal: "X", CreatedAt: now, Status: session.PlanStatusExecuting},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	res := runTasksAction(t, tool, `{"action":"plan_propose"}`)
	if !res.IsError {
		t.Fatalf("expected plan_propose to reject executing→proposed, got %s", res.Content[0].Text)
	}
}

func TestTasks_PlanApproveProposedToExecuting(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	runTasksAction(t, tool, `{"action":"plan_propose"}`)
	runTasksAction(t, tool, `{"action":"plan_approve"}`)

	st, _ := store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusExecuting {
		t.Errorf("expected executing, got %q", st.Plan.Status)
	}
}

func TestTasks_PlanApproveRejectsDrafting(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	res := runTasksAction(t, tool, `{"action":"plan_approve"}`)
	if !res.IsError {
		t.Fatalf("expected plan_approve to reject drafting→executing, got %s", res.Content[0].Text)
	}
	if !strings.Contains(res.Content[0].Text, "drafting") {
		t.Errorf("expected error to mention current status %q, got %q", "drafting", res.Content[0].Text)
	}
}

func TestTasks_AutoExecutingOnFirstInProgress(t *testing.T) {
	// LLMs can forget plan_approve; the first task transition to in_progress
	// should still flip a "proposed" plan into "executing" so the system
	// state stays coherent.
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	runTasksAction(t, tool, `{"action":"add","title":"step 1"}`)
	runTasksAction(t, tool, `{"action":"plan_propose"}`)

	// Confirm pre-state.
	st, _ := store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusProposed {
		t.Fatalf("setup error: expected proposed before update, got %q", st.Plan.Status)
	}

	// Move task to in_progress without calling plan_approve.
	runTasksAction(t, tool, `{"action":"update","id":"1","status":"in_progress"}`)

	st, _ = store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusExecuting {
		t.Errorf("expected auto-transition to executing, got %q", st.Plan.Status)
	}
}

func TestTasks_AutoCompletedWhenAllTasksDone(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	runTasksAction(t, tool, `{"action":"add","title":"a"}`)
	runTasksAction(t, tool, `{"action":"add","title":"b"}`)
	runTasksAction(t, tool, `{"action":"plan_propose"}`)
	runTasksAction(t, tool, `{"action":"plan_approve"}`)
	runTasksAction(t, tool, `{"action":"update","id":"1","status":"completed"}`)

	st, _ := store.GetTasks(sid)
	if st.Plan.Status == session.PlanStatusCompleted {
		t.Fatal("plan went to completed too early — task 2 still pending")
	}

	runTasksAction(t, tool, `{"action":"update","id":"2","status":"cancelled"}`)
	st, _ = store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusCompleted {
		t.Errorf("expected plan to auto-complete after every task done, got %q", st.Plan.Status)
	}
}

func TestTasks_PlanPauseAndResume(t *testing.T) {
	tool, sid, store := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	runTasksAction(t, tool, `{"action":"plan_propose"}`)
	runTasksAction(t, tool, `{"action":"plan_approve"}`)

	if res := runTasksAction(t, tool, `{"action":"plan_pause"}`); res.IsError {
		t.Fatalf("plan_pause: %s", res.Content[0].Text)
	}
	st, _ := store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusPaused {
		t.Fatalf("expected paused, got %q", st.Plan.Status)
	}

	if res := runTasksAction(t, tool, `{"action":"plan_resume"}`); res.IsError {
		t.Fatalf("plan_resume: %s", res.Content[0].Text)
	}
	st, _ = store.GetTasks(sid)
	if st.Plan.Status != session.PlanStatusExecuting {
		t.Errorf("expected executing after resume, got %q", st.Plan.Status)
	}
}

func TestTasks_PlanPauseRejectsDrafting(t *testing.T) {
	tool, _, _ := newTasksToolForTest(t)
	runTasksAction(t, tool, `{"action":"plan_set","goal":"X"}`)
	res := runTasksAction(t, tool, `{"action":"plan_pause"}`)
	if !res.IsError {
		t.Fatalf("expected plan_pause to reject drafting→paused, got %s", res.Content[0].Text)
	}
}

func TestTasks_PlanAbortFromAnyActiveState(t *testing.T) {
	// plan_abort is a panic button that should work from drafting,
	// proposed, executing, and paused — but not from completed/aborted.
	for _, from := range []string{
		session.PlanStatusDrafting,
		session.PlanStatusProposed,
		session.PlanStatusExecuting,
		session.PlanStatusPaused,
	} {
		t.Run(from, func(t *testing.T) {
			tool, sid, store := newTasksToolForTest(t)
			now := session.NowRFC3339()
			if err := store.SaveTasks(sid, session.SessionTasks{
				Plan: &session.Plan{Goal: "X", CreatedAt: now, Status: from},
			}); err != nil {
				t.Fatalf("save: %v", err)
			}
			res := runTasksAction(t, tool, `{"action":"plan_abort"}`)
			if res.IsError {
				t.Fatalf("plan_abort from %s failed: %s", from, res.Content[0].Text)
			}
			st, _ := store.GetTasks(sid)
			if st.Plan.Status != session.PlanStatusAborted {
				t.Errorf("expected aborted from %s, got %q", from, st.Plan.Status)
			}
		})
	}
}

func TestTasks_PlanAbortRejectsTerminal(t *testing.T) {
	for _, terminal := range []string{session.PlanStatusCompleted, session.PlanStatusAborted} {
		t.Run(terminal, func(t *testing.T) {
			tool, sid, store := newTasksToolForTest(t)
			now := session.NowRFC3339()
			if err := store.SaveTasks(sid, session.SessionTasks{
				Plan: &session.Plan{Goal: "X", CreatedAt: now, Status: terminal},
			}); err != nil {
				t.Fatalf("save: %v", err)
			}
			res := runTasksAction(t, tool, `{"action":"plan_abort"}`)
			if !res.IsError {
				t.Errorf("expected plan_abort to reject %s, got success", terminal)
			}
		})
	}
}

func TestTasks_PlanTransitionWithoutPlan(t *testing.T) {
	// All transition actions must surface a clear error when no plan exists
	// rather than creating one implicitly.
	tool, _, _ := newTasksToolForTest(t)
	for _, action := range []string{"plan_propose", "plan_approve", "plan_pause", "plan_resume", "plan_abort"} {
		body := `{"action":"` + action + `"}`
		res := runTasksAction(t, tool, body)
		if !res.IsError {
			t.Errorf("expected %s without plan to error, got %s", action, res.Content[0].Text)
		}
	}
}
