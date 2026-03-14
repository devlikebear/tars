package project

import (
	"context"
	"testing"
	"time"
)

func TestParseTaskReport(t *testing.T) {
	report := ParseTaskReport(`
noise
<task-report>
status: approved
summary: looks good
tests: go test ./...
build: go test ./...
notes: ship it
</task-report>
`)
	if report.Status != "approved" {
		t.Fatalf("expected approved status, got %+v", report)
	}
	if report.Summary != "looks good" {
		t.Fatalf("expected summary to be parsed, got %+v", report)
	}
}

func TestOrchestratorDispatchReviewApprovesTaskToDone(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Review Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Review orchestrator output",
				Status:         "review",
				Assignee:       "dev-1",
				Role:           "developer",
				WorkerKind:     WorkerKindCodexCLI,
				ReviewRequired: true,
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      WorkerKindClaudeCode,
		WorkerKind: WorkerKindClaudeCode,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: approved
summary: ok
tests: go test ./internal/project
build: go test ./internal/gateway
notes: approved
</task-report>`,
	}
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchReview(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected review run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch review: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected approved task to move to done, got %+v", board.Tasks)
	}
	if board.Tasks[0].ReviewApprovedBy != WorkerKindClaudeCode {
		t.Fatalf("expected reviewer %q, got %+v", WorkerKindClaudeCode, board.Tasks[0])
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindReviewStatus, "approved") {
		t.Fatalf("expected approved review activity, got %+v", activity)
	}
}

func TestOrchestratorDispatchReviewRejectsTaskBackToInProgress(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Reject Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Review orchestrator output",
				Status:         "review",
				Assignee:       "dev-1",
				Role:           "developer",
				WorkerKind:     WorkerKindCodexCLI,
				ReviewRequired: true,
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      WorkerKindClaudeCode,
		WorkerKind: WorkerKindClaudeCode,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: rejected
summary: not ready
tests: go test ./internal/project
build: go test ./internal/gateway
notes: fix the failing case
</task-report>`,
	}
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchReview(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected review run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch review: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "in_progress" {
		t.Fatalf("expected rejected task to move to in_progress, got %+v", board.Tasks)
	}
	if board.Tasks[0].ReviewApprovedBy != "" {
		t.Fatalf("expected review approver to be cleared, got %+v", board.Tasks[0])
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindReviewStatus, "rejected") {
		t.Fatalf("expected rejected review activity, got %+v", activity)
	}
}

func hasActivityKindStatus(items []Activity, kind, status string) bool {
	for _, item := range items {
		if item.Kind == kind && item.Status == status {
			return true
		}
	}
	return false
}
