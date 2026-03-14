package project

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorDispatchTodoBlocksReviewWhenVerificationFails(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 16, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Verification Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Implement verification gate",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/gateway",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      WorkerKindCodexCLI,
		WorkerKind: WorkerKindCodexCLI,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: implemented
tests: failed
build: passed
issue: https://github.com/devlikebear/tars/issues/201
branch: feat/task-1
pr: https://github.com/devlikebear/tars/pull/301
notes: test still failing
</task-report>`,
	}
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err == nil {
		t.Fatal("expected verification gate error")
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "in_progress" {
		t.Fatalf("expected verification failure to hold task in progress, got %+v", board.Tasks)
	}
	if board.Tasks[0].Issue == "" || board.Tasks[0].Branch == "" || board.Tasks[0].PR == "" {
		t.Fatalf("expected github flow metadata from report, got %+v", board.Tasks[0])
	}

	activity, err := store.ListActivity(created.ID, 30)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindTestStatus, "failed") {
		t.Fatalf("expected failed test activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindBuildStatus, "passed") {
		t.Fatalf("expected passed build activity, got %+v", activity)
	}
}

func TestOrchestratorDispatchTodoFailsWhenGitHubAuthMissing(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 16, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Auth Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Implement auth gate",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/gateway",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error {
		return context.DeadlineExceeded
	})

	_, err = orchestrator.DispatchTodo(context.Background(), created.ID)
	if err == nil {
		t.Fatal("expected github auth error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "github") && !strings.Contains(strings.ToLower(err.Error()), "gh auth") {
		t.Fatalf("expected github auth error, got %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "todo" {
		t.Fatalf("expected auth failure to leave task in todo, got %+v", board.Tasks)
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindIssueStatus, "blocked") {
		t.Fatalf("expected blocked issue status activity, got %+v", activity)
	}
}
