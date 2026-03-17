package project

import (
	"context"
	"os"
	"path/filepath"
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

type recoveringTaskRunner struct {
	attempts map[string]int
}

func (r *recoveringTaskRunner) Start(_ context.Context, req TaskRunRequest) (TaskRun, error) {
	if r.attempts == nil {
		r.attempts = map[string]int{}
	}
	return TaskRun{
		ID:         "run-" + req.Role + "-" + req.TaskID,
		TaskID:     req.TaskID,
		Agent:      req.Agent,
		WorkerKind: req.WorkerKind,
		Status:     TaskRunStatusAccepted,
	}, nil
}

func (r *recoveringTaskRunner) Wait(_ context.Context, runID string) (TaskRun, error) {
	if strings.Contains(runID, "reviewer") {
		return TaskRun{
			ID:         runID,
			TaskID:     "task-1",
			Agent:      WorkerKindClaudeCode,
			WorkerKind: WorkerKindClaudeCode,
			Status:     TaskRunStatusCompleted,
			Response: `<task-report>
status: approved
summary: approved after retry
tests: passed
build: passed
notes: approved
</task-report>`,
		}, nil
	}
	if r.attempts == nil {
		r.attempts = map[string]int{}
	}
	r.attempts[runID]++
	if r.attempts[runID] == 1 {
		return TaskRun{
			ID:         runID,
			TaskID:     "task-1",
			Agent:      "dev-1",
			WorkerKind: WorkerKindCodexCLI,
			Status:     TaskRunStatusCompleted,
			Response: `<task-report>
status: completed
summary: first attempt missed github flow metadata
tests: passed
build: passed
notes: blocked on missing metadata
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
summary: retried with metadata
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/57
branch: fix/task-1
pr: https://github.com/devlikebear/tars/pull/57
notes: ok
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

func TestAutopilotManager_StartRecoversVerificationGateFailureAndCompletes(t *testing.T) {
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

	manager := NewAutopilotManager(store, &recoveringTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.loopInterval = 5 * time.Millisecond
	if _, err := manager.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)
	if final.Iterations < 3 {
		t.Fatalf("expected retry iterations before completion, got %+v", final)
	}
	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "done" || state.Phase != "done" {
		t.Fatalf("expected recovered autopilot to finish, got %+v", state)
	}

	activity, err := store.ListActivity(created.ID, 100)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindDecision, "auto_retry") {
		t.Fatalf("expected auto-retry decision activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindReplan, "applied") {
		t.Fatalf("expected applied replan activity, got %+v", activity)
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

func TestAutopilotManager_StartRecoversStalledInProgressTaskAndCompletes(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 18, 50, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Stalled"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Recover stalled task",
				Status:         "in_progress",
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
	manager.loopInterval = 5 * time.Millisecond
	if _, err := manager.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)
	if final.Iterations < 3 {
		t.Fatalf("expected recovery iteration before completion, got %+v", final)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected recovered task to finish, got %+v", board.Tasks)
	}

	activity, err := store.ListActivity(created.ID, 100)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindDecision, "auto_retry") {
		t.Fatalf("expected auto-retry decision activity, got %+v", activity)
	}
	if !hasActivityKindStatus(activity, ActivityKindReplan, "applied") {
		t.Fatalf("expected applied replan activity, got %+v", activity)
	}
}

func TestAutopilotManager_StatusRestoresPersistedRunAfterRestart(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 19, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Persisted"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Ship persisted task",
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
	if _, err := manager.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}
	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)

	restarted := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	restored, ok := restarted.Status(created.ID)
	if !ok {
		t.Fatalf("expected persisted autopilot status after restart")
	}
	if restored.Status != final.Status {
		t.Fatalf("expected restored status %q, got %+v", final.Status, restored)
	}
	if restored.RunID != final.RunID {
		t.Fatalf("expected restored run id %q, got %+v", final.RunID, restored)
	}
}

func TestAutopilotManager_StatusKeepsPersistedRunningRunAfterRestart(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 19, 15, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Interrupted"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-interrupted"
		item.Status = AutopilotStatusRunning
		item.Message = "Dispatching todo tasks"
		item.Iterations = 3
		item.StartedAt = "2026-03-14T10:20:56Z"
	})

	restarted := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	restored, ok := restarted.Status(created.ID)
	if !ok {
		t.Fatalf("expected persisted interrupted autopilot status")
	}
	if restored.Status != AutopilotStatusRunning {
		t.Fatalf("expected persisted running status to remain visible until recovery restarts it, got %+v", restored)
	}
}

func TestAutopilotManager_RestorePersistedRunsPreloadsCompletedRuns(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 19, 30, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Preload"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-finished"
		item.Status = AutopilotStatusDone
		item.Message = "Autopilot completed all project tasks"
		item.Iterations = 4
		item.StartedAt = "2026-03-14T10:20:56Z"
		item.FinishedAt = "2026-03-14T10:30:56Z"
	})

	restarted := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	if err := restarted.RestorePersistedRuns(); err != nil {
		t.Fatalf("restore persisted runs: %v", err)
	}

	restarted.mu.RLock()
	restored, ok := restarted.runs[created.ID]
	restarted.mu.RUnlock()
	if !ok {
		t.Fatalf("expected restored run to be preloaded into manager cache")
	}
	if restored.Status != AutopilotStatusDone {
		t.Fatalf("expected restored done status, got %+v", restored)
	}
}

func TestAutopilotManager_RestorePersistedRunsRestartsRunningRunsAtStartup(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 19, 45, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Startup Recovery"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	initialPhase := "executing"
	initialStatus := "active"
	initialNextAction := "Dispatch todo tasks"
	if _, err := store.UpdateState(created.ID, ProjectStateUpdateInput{
		Phase:      &initialPhase,
		Status:     &initialStatus,
		NextAction: &initialNextAction,
	}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-running"
		item.Status = AutopilotStatusRunning
		item.Message = "Dispatching todo tasks"
		item.Iterations = 2
		item.StartedAt = "2026-03-14T10:20:56Z"
	})

	restarted := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	restarted.loopInterval = 5 * time.Millisecond
	if err := restarted.RestorePersistedRuns(); err != nil {
		t.Fatalf("restore persisted runs: %v", err)
	}

	restored := waitForAutopilotStatus(t, restarted, created.ID, AutopilotStatusDone)
	if restored.Iterations < 1 {
		t.Fatalf("expected restarted loop to advance the project, got %+v", restored)
	}

	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "done" || state.Phase != "done" {
		t.Fatalf("expected startup recovery to resume and finish, got %+v", state)
	}

	activity, err := store.ListActivity(created.ID, 20)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if !hasActivityKindStatus(activity, ActivityKindReviewStatus, "approved") {
		t.Fatalf("expected restart recovery to finish the recovered workflow, got %+v", activity)
	}
}

func TestAutopilotManager_RestorePersistedRunsRestartsIncompleteProjects(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 19, 45, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Resume Active"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Resume this project",
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
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-resume"
		item.Status = AutopilotStatusBlocked
		item.Message = "Autopilot stalled before restart"
		item.Iterations = 1
		item.StartedAt = "2026-03-14T10:20:56Z"
	})

	restarted := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	restarted.loopInterval = 5 * time.Millisecond
	if err := restarted.RestorePersistedRuns(); err != nil {
		t.Fatalf("restore persisted runs: %v", err)
	}

	final := waitForAutopilotStatus(t, restarted, created.ID, AutopilotStatusDone)
	if final.Iterations < 2 {
		t.Fatalf("expected restored loop to continue running, got %+v", final)
	}
}

func TestAutopilotManager_EnsureActiveRunsStartsMissingLoop(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 20, 5, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Ensure Active"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Force start missing loop",
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
	manager.loopInterval = 5 * time.Millisecond
	started, err := manager.EnsureActiveRuns(context.Background())
	if err != nil {
		t.Fatalf("ensure active runs: %v", err)
	}
	if started != 1 {
		t.Fatalf("expected one started run, got %d", started)
	}

	final := waitForAutopilotStatus(t, manager, created.ID, AutopilotStatusDone)
	if final.Status != AutopilotStatusDone {
		t.Fatalf("expected done status, got %+v", final)
	}
}

func TestAutopilotManager_SetRunPersistsAutopilotFileWithoutTempArtifacts(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 14, 20, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Autopilot Atomic"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-atomic"
		item.Status = AutopilotStatusBlocked
		item.Message = "Autopilot blocked"
	})

	if _, err := os.Stat(store.ProjectFilePath(created.ID, autopilotRunDocumentName)); err != nil {
		t.Fatalf("expected persisted autopilot file: %v", err)
	}
	tmpFiles, err := filepath.Glob(store.ProjectFilePath(created.ID, "."+autopilotRunDocumentName+".tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(tmpFiles) != 0 {
		t.Fatalf("expected no temp artifacts after atomic persist, got %v", tmpFiles)
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
