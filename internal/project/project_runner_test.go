package project

import (
	"context"
	"strings"
	"testing"
	"time"
)

type stagedTaskRunner struct{}

func (stagedTaskRunner) Start(_ context.Context, req TaskRunRequest) (TaskRun, error) {
	runID := "run-" + req.Role + "-" + req.TaskID
	return TaskRun{
		ID:         runID,
		TaskID:     req.TaskID,
		Agent:      req.Agent,
		WorkerKind: req.WorkerKind,
		Status:     TaskRunStatusAccepted,
	}, nil
}

func (stagedTaskRunner) Wait(_ context.Context, runID string) (TaskRun, error) {
	if strings.Contains(runID, "reviewer") {
		return TaskRun{
			ID:         runID,
			TaskID:     "task-1",
			Agent:      WorkerKindClaudeCode,
			WorkerKind: WorkerKindClaudeCode,
			Status:     TaskRunStatusCompleted,
			Response: `<task-report>
status: approved
summary: approved
tests: go test ./internal/project
build: go test ./internal/tool
notes: approved
</task-report>`,
		}, nil
	}
	return TaskRun{
		ID:         runID,
		TaskID:     "task-1",
		Agent:      "dev-1",
		WorkerKind: WorkerKindCodexCLI,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: implemented
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/1
branch: feat/task-1
pr: https://github.com/devlikebear/tars/pull/1
notes: ok
</task-report>`,
	}, nil
}

type blockingTaskRunner struct{}

func (blockingTaskRunner) Start(_ context.Context, req TaskRunRequest) (TaskRun, error) {
	return TaskRun{
		ID:         "run-" + req.TaskID,
		TaskID:     req.TaskID,
		Agent:      req.Agent,
		WorkerKind: req.WorkerKind,
		Status:     TaskRunStatusAccepted,
	}, nil
}

func (blockingTaskRunner) Wait(_ context.Context, runID string) (TaskRun, error) {
	return TaskRun{
		ID:         runID,
		TaskID:     "task-1",
		Agent:      "dev-1",
		WorkerKind: WorkerKindCodexCLI,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: missing github metadata
tests: passed
build: passed
notes: blocked
</task-report>`,
	}, nil
}

func TestAutopilotManager_StartCompletesTodoAndReviewFlow(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 18, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Done"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Ship todo app MVP",
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

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	started, err := manager.Start(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("start autopilot: %v", err)
	}
	if started.Status != AutopilotStatusRunning {
		t.Fatalf("expected running status, got %+v", started)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)
	if final.Iterations < 2 {
		t.Fatalf("expected at least 2 iterations for todo+review flow, got %+v", final)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected done task, got %+v", board.Tasks)
	}

	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "done" || state.Phase != "done" {
		t.Fatalf("expected done state, got %+v", state)
	}
}

func TestAutopilotManager_StartBlocksWhenVerificationGateFails(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 18, 30, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Blocked"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:           "task-1",
				Title:        "Ship todo app MVP",
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

	manager := NewAutopilotManager(store, blockingTaskRunner{}, func(context.Context) error { return nil }, nil)
	if _, err := manager.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusBlocked)
	if !strings.Contains(strings.ToLower(final.Message), "blocked") {
		t.Fatalf("expected blocked message, got %+v", final)
	}

	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "blocked" || state.Phase != "blocked" {
		t.Fatalf("expected blocked state, got %+v", state)
	}
}

func TestAutopilotManager_StartBlocksWhenBoardIsEmpty(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 18, 45, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Empty"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	if _, err := manager.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)
	if final.Iterations < 1 {
		t.Fatalf("expected autopilot to iterate after seeding backlog, got %+v", final)
	}

	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "done" || state.Phase != "done" {
		t.Fatalf("expected seeded project to finish, got %+v", state)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) == 0 {
		t.Fatalf("expected pm supervisor to seed backlog, got %+v", board)
	}

	activity, err := store.ListActivity(created.ID, 100)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindReplan, "seeded") {
		t.Fatalf("expected seeded replan activity, got %+v", activity)
	}
}

func waitForAutopilotStatus(t *testing.T, manager *AutopilotManager, projectID string, want AutopilotRunStatus) AutopilotRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		item, ok := manager.Status(projectID)
		if ok && item.Status == want {
			return item
		}
		time.Sleep(20 * time.Millisecond)
	}
	item, _ := manager.Status(projectID)
	t.Fatalf("expected autopilot status %q, got %+v", want, item)
	return AutopilotRun{}
}
