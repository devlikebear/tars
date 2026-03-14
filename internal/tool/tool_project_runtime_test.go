package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/project"
)

type stubProjectTaskRunner struct {
	result project.TaskRun
}

func (s stubProjectTaskRunner) Start(_ context.Context, req project.TaskRunRequest) (project.TaskRun, error) {
	run := s.result
	if run.ID == "" {
		run.ID = "run-" + req.TaskID
	}
	if run.TaskID == "" {
		run.TaskID = req.TaskID
	}
	if run.Agent == "" {
		run.Agent = req.Agent
	}
	if run.WorkerKind == "" {
		run.WorkerKind = req.WorkerKind
	}
	if run.Status == "" {
		run.Status = project.TaskRunStatusAccepted
	}
	return run, nil
}

func (s stubProjectTaskRunner) Wait(_ context.Context, runID string) (project.TaskRun, error) {
	run := s.result
	if run.ID == "" {
		run.ID = runID
	}
	if run.Status == "" || run.Status == project.TaskRunStatusAccepted {
		run.Status = project.TaskRunStatusCompleted
	}
	if run.Response == "" {
		run.Response = `<task-report>
status: completed
summary: ok
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/1
branch: feat/task-1
pr: https://github.com/devlikebear/tars/pull/1
notes: ok
</task-report>`
	}
	return run, nil
}

func TestProjectBoardTools_UpdateAndGet(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), nil)
	created, err := store.Create(project.CreateInput{Name: "Board Tool Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	updateTool := NewProjectBoardUpdateTool(store)
	result, err := updateTool.Execute(context.Background(), json.RawMessage(`{
		"project_id":"`+created.ID+`",
		"tasks":[
			{
				"id":"task-1",
				"title":"Implement kickoff skill",
				"status":"todo",
				"assignee":"dev-1",
				"role":"developer",
				"review_required":true
			}
		]
	}`))
	if err != nil {
		t.Fatalf("execute project_board_update: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	getTool := NewProjectBoardGetTool(store)
	result, err = getTool.Execute(context.Background(), json.RawMessage(`{"project_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("execute project_board_get: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var board project.Board
	if err := json.Unmarshal([]byte(result.Text()), &board); err != nil {
		t.Fatalf("decode board: %v", err)
	}
	if board.ProjectID != created.ID {
		t.Fatalf("expected project id %q, got %+v", created.ID, board)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].ID != "task-1" {
		t.Fatalf("expected seeded task, got %+v", board.Tasks)
	}
}

func TestProjectActivityTools_AppendAndGet(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), func() time.Time {
		return time.Date(2026, 3, 14, 16, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(project.CreateInput{Name: "Activity Tool Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	appendTool := NewProjectActivityAppendTool(store)
	result, err := appendTool.Execute(context.Background(), json.RawMessage(`{
		"project_id":"`+created.ID+`",
		"task_id":"task-1",
		"source":"pm",
		"kind":"assignment",
		"status":"queued",
		"message":"Assign task-1 to dev-1"
	}`))
	if err != nil {
		t.Fatalf("execute project_activity_append: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	getTool := NewProjectActivityGetTool(store)
	result, err = getTool.Execute(context.Background(), json.RawMessage(`{"project_id":"`+created.ID+`","limit":10}`))
	if err != nil {
		t.Fatalf("execute project_activity_get: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var payload struct {
		Count int                `json:"count"`
		Items []project.Activity `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode activity payload: %v", err)
	}
	if payload.Count == 0 || len(payload.Items) == 0 {
		t.Fatalf("expected activity items, got %+v", payload)
	}
	if payload.Items[0].Kind != "assignment" {
		t.Fatalf("expected assignment activity, got %+v", payload.Items[0])
	}
}

func TestProjectDispatchTool_DispatchesTodoTasks(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), func() time.Time {
		return time.Date(2026, 3, 14, 17, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(project.CreateInput{Name: "Dispatch Tool Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, project.BoardUpdateInput{
		Tasks: []project.BoardTask{
			{
				ID:             "task-1",
				Title:          "Run developer task",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tool",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	dispatchTool := NewProjectDispatchTool(store, stubProjectTaskRunner{}, func(context.Context) error { return nil })
	result, err := dispatchTool.Execute(context.Background(), json.RawMessage(`{
		"project_id":"`+created.ID+`",
		"stage":"todo"
	}`))
	if err != nil {
		t.Fatalf("execute project_dispatch: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var report project.DispatchReport
	if err := json.Unmarshal([]byte(result.Text()), &report); err != nil {
		t.Fatalf("decode dispatch report: %v", err)
	}
	if report.ProjectID != created.ID || len(report.Runs) != 1 {
		t.Fatalf("unexpected dispatch report: %+v", report)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "review" {
		t.Fatalf("expected task to move to review, got %+v", board.Tasks)
	}
}

func TestProjectAutopilotStartTool_StartsBackgroundRunner(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), func() time.Time {
		return time.Date(2026, 3, 14, 17, 30, 0, 0, time.UTC)
	})
	created, err := store.Create(project.CreateInput{Name: "Autopilot Tool Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, project.BoardUpdateInput{
		Tasks: []project.BoardTask{
			{
				ID:           "task-1",
				Title:        "Run developer task",
				Status:       "todo",
				Assignee:     "dev-1",
				Role:         "developer",
				TestCommand:  "go test ./internal/project",
				BuildCommand: "go test ./internal/tool",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	manager := project.NewAutopilotManager(store, stubProjectTaskRunner{}, func(context.Context) error { return nil }, nil)
	tool := NewProjectAutopilotStartTool(manager)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"project_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("execute project_autopilot_start: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var run project.AutopilotRun
	if err := json.Unmarshal([]byte(result.Text()), &run); err != nil {
		t.Fatalf("decode autopilot run: %v", err)
	}
	if run.ProjectID != created.ID || run.Status != project.AutopilotStatusRunning {
		t.Fatalf("unexpected autopilot run: %+v", run)
	}

	final := waitForAutopilotToFinish(t, manager, created.ID)
	if final.Status != project.AutopilotStatusDone {
		t.Fatalf("expected autopilot to finish with done status, got %+v", final)
	}
}

func waitForAutopilotToFinish(t *testing.T, manager *project.AutopilotManager, projectID string) project.AutopilotRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		item, ok := manager.Status(projectID)
		if ok && item.Status != project.AutopilotStatusRunning {
			return item
		}
		time.Sleep(20 * time.Millisecond)
	}
	item, _ := manager.Status(projectID)
	t.Fatalf("expected autopilot run to finish, got %+v", item)
	return project.AutopilotRun{}
}
