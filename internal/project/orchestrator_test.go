package project

import (
	"context"
	"fmt"
	"strings"
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

func TestOrchestratorDispatchTodo_DoesNotSpecialCaseLegacySeedTaskIDs(t *testing.T) {
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
		t.Fatal("expected first legacy-seed task run to start")
	}
	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected second legacy-seed task to start without special-casing")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.started) != 2 {
		t.Fatalf("expected both legacy-seed tasks to be dispatched, got %+v", runner.started)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %+v", board.Tasks)
	}
	if board.Tasks[0].Status != "review" || board.Tasks[1].Status != "review" {
		t.Fatalf("expected both legacy-seed tasks to advance, got %+v", board.Tasks)
	}
}

func TestOrchestratorDispatchTodo_CanonicalizesLegacyKanbanColumnsDuringRun(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 20, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Legacy Kanban Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Columns: []string{"backlog", "todo", "doing", "review", "done"},
		Tasks: []BoardTask{
			{ID: "task-1", Title: "Build kickoff slice", Status: "todo", Assignee: "dev-1", Role: "developer", ReviewRequired: true},
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
		t.Fatal("expected task run to start")
	}

	boardDuringRun, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board during run: %v", err)
	}
	if len(boardDuringRun.Tasks) != 1 || boardDuringRun.Tasks[0].Status != "in_progress" {
		t.Fatalf("expected canonical in_progress status during run, got %+v", boardDuringRun.Tasks)
	}
	if len(boardDuringRun.Columns) != 4 || boardDuringRun.Columns[0] != "todo" || boardDuringRun.Columns[1] != "in_progress" {
		t.Fatalf("expected canonical columns during run, got %+v", boardDuringRun.Columns)
	}

	close(runner.waitGate)
	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}
}

func TestOrchestratorDispatchTodo_CompletesTaskWithoutReviewRequirement(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 30, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Done Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Ship metrics page",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: false,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
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
		t.Fatal("expected task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected task to move to done, got %+v", board.Tasks)
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasTaskStatusActivity(activity, "task-1", "done") {
		t.Fatalf("expected done task activity, got %+v", activity)
	}
}

func TestOrchestratorDispatchTodo_BlocksTaskWhenGitHubFlowMetadataMissing(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 35, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Blocked Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Ship metrics page",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: false,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      "dev-1",
		WorkerKind: WorkerKindCodexCLI,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: ok
tests: passed
build: passed
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

	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "task gate failed") {
		t.Fatalf("expected task gate failed error, got %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "in_progress" {
		t.Fatalf("expected task to remain in_progress when gates fail, got %+v", board.Tasks)
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasTaskStatusActivity(activity, "task-1", "in_progress") {
		t.Fatalf("expected in_progress task activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindIssueStatus, "blocked") {
		t.Fatalf("expected blocked issue status activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindBranchStatus, "blocked") {
		t.Fatalf("expected blocked branch status activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindPRStatus, "blocked") {
		t.Fatalf("expected blocked pr status activity, got %+v", activity)
	}
}

func TestOrchestratorDispatchTodo_ResearchProfileSkipsSoftwareDevGates(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 37, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{
		Name:            "Research Project",
		WorkflowProfile: "research",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Summarize recent papers",
				Status:         "todo",
				Assignee:       "analyst-1",
				Role:           "developer",
				ReviewRequired: false,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      "analyst-1",
		WorkerKind: WorkerKindDefault,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: summarized sources
tests: not run
build: not run
notes: ready
</task-report>`,
	}
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error {
		t.Fatal("did not expect github auth checker for research profile")
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected research task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch todo: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected research task to move to done, got %+v", board.Tasks)
	}
	if board.Tasks[0].WorkerKind != WorkerKindDefault {
		t.Fatalf("expected runtime default worker, got %+v", board.Tasks[0])
	}
}

func TestOrchestratorDispatchTodo_WorkflowRulesCanRequireMetadataOutsideSoftwareDev(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 39, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{
		Name:            "Research Gated Project",
		WorkflowProfile: "research",
		WorkflowRules: []WorkflowRule{
			{Name: "require_issue"},
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Summarize recent papers",
				Status:         "todo",
				Assignee:       "analyst-1",
				Role:           "developer",
				ReviewRequired: false,
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	runner.results["run-task-1"] = TaskRun{
		ID:         "run-task-1",
		TaskID:     "task-1",
		Agent:      "analyst-1",
		WorkerKind: WorkerKindDefault,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: summarized sources
notes: ready
</task-report>`,
	}
	orchestrator := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error {
		t.Fatal("did not expect github auth checker for research metadata rule")
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orchestrator.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected gated research task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "task gate failed") {
		t.Fatalf("expected task gate failure, got %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "in_progress" {
		t.Fatalf("expected workflow rule to keep task in_progress, got %+v", board.Tasks)
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

// stubSkillResolver implements SkillResolver for testing.
type stubSkillResolver struct {
	skills []SkillContent
}

func (s *stubSkillResolver) ResolveSkills(_ []string) []SkillContent {
	return s.skills
}

func TestOrchestratorDispatchTodo_InjectsProjectSkillsIntoPrompt(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{
		Name:        "Skill Project",
		SkillsAllow: []string{"github-dev"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{ID: "task-1", Title: "Create issue", Status: "todo", Assignee: "planner", Role: "developer", ReviewRequired: true},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	orch := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })
	orch.SetSkillResolver(&stubSkillResolver{
		skills: []SkillContent{
			{Name: "github-dev", Content: "Always use `gh issue create` for issue creation."},
		},
	})

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orch.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.started) != 1 {
		t.Fatalf("expected 1 started task, got %d", len(runner.started))
	}
	prompt := runner.started[0].Prompt
	if !strings.Contains(prompt, "## Project Skills") {
		t.Fatalf("expected skills section in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "### github-dev") {
		t.Fatalf("expected github-dev skill header in prompt")
	}
	if !strings.Contains(prompt, "gh issue create") {
		t.Fatalf("expected skill content in prompt")
	}
}

func TestOrchestratorDispatchTodo_NoSkillsWhenResolverNil(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 13, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{
		Name:        "No Skill Project",
		SkillsAllow: []string{"github-dev"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{ID: "task-1", Title: "Do work", Status: "todo", Assignee: "dev", Role: "developer", ReviewRequired: true},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := newStubTaskRunner()
	orch := NewOrchestratorWithGitHubAuthChecker(store, runner, func(context.Context) error { return nil })
	// No SetSkillResolver — resolver is nil

	errCh := make(chan error, 1)
	go func() {
		_, runErr := orch.DispatchTodo(context.Background(), created.ID)
		errCh <- runErr
	}()

	select {
	case <-runner.startedCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected task run to start")
	}
	close(runner.waitGate)

	if err := <-errCh; err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	prompt := runner.started[0].Prompt
	if strings.Contains(prompt, "## Project Skills") {
		t.Fatalf("expected no skills section when resolver is nil, got:\n%s", prompt)
	}
}
