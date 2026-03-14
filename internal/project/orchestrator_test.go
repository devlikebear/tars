package project

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type stubTaskRunner struct {
	mu        sync.Mutex
	started   []TaskRunRequest
	startedCh chan struct{}
	waitGate  chan struct{}
	results   map[string]TaskRun
}

func newStubTaskRunner() *stubTaskRunner {
	return &stubTaskRunner{
		startedCh: make(chan struct{}, 8),
		waitGate:  make(chan struct{}),
		results:   map[string]TaskRun{},
	}
}

func (s *stubTaskRunner) Start(_ context.Context, req TaskRunRequest) (TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = append(s.started, req)
	runID := fmt.Sprintf("run-%s", req.TaskID)
	if _, ok := s.results[runID]; !ok {
		s.results[runID] = TaskRun{
			ID:         runID,
			TaskID:     req.TaskID,
			Agent:      req.Agent,
			WorkerKind: req.WorkerKind,
			Status:     TaskRunStatusCompleted,
			Response: `<task-report>
status: completed
summary: ok
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/1
branch: feat/` + req.TaskID + `
pr: https://github.com/devlikebear/tars/pull/1
notes: ok
</task-report>`,
		}
	}
	s.startedCh <- struct{}{}
	return TaskRun{
		ID:     runID,
		TaskID: req.TaskID,
		Agent:  req.Agent,
		Status: TaskRunStatusAccepted,
	}, nil
}

func (s *stubTaskRunner) Wait(_ context.Context, runID string) (TaskRun, error) {
	<-s.waitGate
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.results[runID], nil
}

func TestOrchestratorDispatchTodoRunsTasksInParallel(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Parallel Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{ID: "task-1", Title: "Build dashboard", Status: "todo", Assignee: "dev-1", Role: "developer", ReviewRequired: true},
			{ID: "task-2", Title: "Wire activity log", Status: "todo", Assignee: "dev-2", Role: "developer", ReviewRequired: true},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-runner.startedCh:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected both task runs to start before waits unblock")
		}
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	for _, task := range board.Tasks {
		if task.Status != "review" {
			t.Fatalf("expected task %s to move to review, got %+v", task.ID, task)
		}
		if task.WorkerKind != WorkerKindCodexCLI {
			t.Fatalf("expected task %s worker kind %q, got %+v", task.ID, WorkerKindCodexCLI, task)
		}
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasTaskStatusActivity(activity, "task-1", "review") || !hasTaskStatusActivity(activity, "task-2", "review") {
		t.Fatalf("expected completion activity for both tasks, got %+v", activity)
	}
	if !hasTaskStatusMeta(activity, "task-1", "worker_kind", WorkerKindCodexCLI) {
		t.Fatalf("expected task-1 worker kind activity meta, got %+v", activity)
	}
}

func TestOrchestratorDispatchTodo_StagesBootstrapBeforeDependentSeedTasks(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 15, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Seeded PM Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{ID: "pm-seed-bootstrap", Title: "Bootstrap MVP", Status: "todo", Assignee: "dev-1", Role: "developer", ReviewRequired: true},
			{ID: "pm-seed-vertical-slice", Title: "Implement first vertical slice", Status: "todo", Assignee: "dev-2", Role: "developer", ReviewRequired: true},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected bootstrap task run to start")
	}
	select {
	case <-runner.startedCh:
		t.Fatal("expected seeded dependent task to stay queued until bootstrap finishes")
	case <-time.After(120 * time.Millisecond):
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.started) != 1 {
		t.Fatalf("expected only bootstrap task to be dispatched, got %+v", runner.started)
	}
	if runner.started[0].TaskID != "pm-seed-bootstrap" {
		t.Fatalf("expected bootstrap task first, got %+v", runner.started[0])
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %+v", board.Tasks)
	}
	if board.Tasks[0].Status != "review" {
		t.Fatalf("expected bootstrap task to advance, got %+v", board.Tasks[0])
	}
	if board.Tasks[1].Status != "todo" {
		t.Fatalf("expected dependent task to remain queued, got %+v", board.Tasks[1])
	}
}

func TestOrchestratorDispatchTodoRestoresFailedTaskToTodo(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Failure Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{ID: "task-1", Title: "Handle failure", Status: "todo", Assignee: "dev-1", Role: "developer"},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:     "run-task-1",
		TaskID: "task-1",
		Agent:  "dev-1",
		Status: TaskRunStatusFailed,
		Error:  "boom",
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
		t.Fatal("expected failed task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err == nil {
		t.Fatal("expected failed task run error")
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "todo" {
		t.Fatalf("expected failed task to return to todo, got %+v", board.Tasks)
	}
	if board.Tasks[0].WorkerKind != WorkerKindCodexCLI {
		t.Fatalf("expected failed task to preserve logical worker kind %q, got %+v", WorkerKindCodexCLI, board.Tasks[0])
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasTaskStatusActivity(activity, "task-1", "failed") {
		t.Fatalf("expected failed task activity, got %+v", activity)
	}
	if !hasAgentReport(activity, "task-1", "failed", "boom") {
		t.Fatalf("expected failed agent report with error details, got %+v", activity)
	}
	if hasActivityKindStatus(activity, ActivityKindTestStatus, "blocked") || hasActivityKindStatus(activity, ActivityKindBuildStatus, "blocked") {
		t.Fatalf("did not expect fake verification statuses for a failed run, got %+v", activity)
	}
}

func hasTaskStatusActivity(items []Activity, taskID, status string) bool {
	for _, item := range items {
		if item.TaskID == taskID && item.Kind == ActivityKindTaskStatus && item.Status == status {
			return true
		}
	}
	return false
}

func hasTaskStatusMeta(items []Activity, taskID, key, want string) bool {
	for _, item := range items {
		if item.TaskID != taskID || item.Kind != ActivityKindTaskStatus {
			continue
		}
		if item.Meta[key] == want {
			return true
		}
	}
	return false
}

func hasAgentReport(items []Activity, taskID, status, errorText string) bool {
	for _, item := range items {
		if item.TaskID != taskID || item.Kind != ActivityKindAgentReport || item.Status != status {
			continue
		}
		if item.Meta["error"] == errorText {
			return true
		}
	}
	return false
}
