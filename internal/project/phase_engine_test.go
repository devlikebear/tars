package project

import (
	"context"
	"sync"
	"testing"
	"time"
)

var _ PhaseEngine = (*AutopilotManager)(nil)

func TestAutopilotManager_CurrentProjectsTypedPhaseSnapshot(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 1, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Phase Snapshot"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	phase := "REVIEWING"
	status := "BLOCKED"
	nextAction := "Wait for human approval"
	lastRunSummary := "Review requested"
	lastRunAt := "2026-03-26T01:00:00Z"
	if _, err := store.UpdateState(created.ID, ProjectStateUpdateInput{
		Phase:          &phase,
		Status:         &status,
		NextAction:     &nextAction,
		LastRunSummary: &lastRunSummary,
		LastRunAt:      &lastRunAt,
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-review"
		item.Status = AutopilotStatusBlocked
		item.Message = "Waiting for review decision"
		item.Iterations = 3
		item.UpdatedAt = "2026-03-26T01:02:00Z"
	})

	current, ok := manager.Current(created.ID)
	if !ok {
		t.Fatal("expected current phase snapshot")
	}
	if current.Name != PhaseReviewing {
		t.Fatalf("expected reviewing phase, got %+v", current)
	}
	if current.Status != PhaseStatusBlocked {
		t.Fatalf("expected blocked phase status, got %+v", current)
	}
	if current.NextAction != nextAction {
		t.Fatalf("expected next action %q, got %+v", nextAction, current)
	}
	if current.Message != "Waiting for review decision" {
		t.Fatalf("expected run message to be projected, got %+v", current)
	}
	if current.RunStatus != AutopilotStatusBlocked {
		t.Fatalf("expected blocked run status, got %+v", current)
	}
}

func TestAutopilotManager_CurrentReturnsFalseWithoutStateOrRun(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 1, 15, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "No State Yet"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	if current, ok := manager.Current(created.ID); ok {
		t.Fatalf("expected current snapshot to be unavailable without state or run, got %+v", current)
	}
}

func TestAutopilotManager_AdvanceRunsSingleStepWithoutStartingLoop(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 1, 30, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Advance Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Advance should start loop",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)

	current, err := manager.Advance(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("advance project: %v", err)
	}
	if current.ProjectID != created.ID {
		t.Fatalf("expected current snapshot for %q, got %+v", created.ID, current)
	}
	if manager.IsRunning(created.ID) {
		t.Fatalf("expected advance to avoid starting the background loop")
	}
	reloaded := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	persisted, ok := reloaded.Status(created.ID)
	if !ok || persisted.Iterations != 1 || persisted.Status != AutopilotStatusRunning {
		t.Fatalf("expected advance step to persist run state, got ok=%v run=%+v", ok, persisted)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board after first advance: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "review" {
		t.Fatalf("expected first advance to move todo -> review, got %+v", board.Tasks)
	}

	if _, err := manager.Advance(context.Background(), created.ID); err != nil {
		t.Fatalf("advance review step: %v", err)
	}
	board, err = store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board after second advance: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "done" {
		t.Fatalf("expected second advance to move review -> done, got %+v", board.Tasks)
	}

	// Multi-phase: after all tasks are done, the third advance clears the board
	// and attempts to plan the next phase. Keep advancing until the autopilot
	// reaches a terminal state (done or blocked) or a non-advancing running state.
	var final PhaseSnapshot
	for i := 0; i < 5; i++ {
		step, err := manager.Advance(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("advance step %d: %v", i+3, err)
		}
		final = step
		if final.RunStatus == AutopilotStatusDone || final.RunStatus == AutopilotStatusBlocked {
			break
		}
	}
	if final.RunStatus != AutopilotStatusBlocked && final.RunStatus != AutopilotStatusDone {
		t.Fatalf("expected project to reach terminal state after advancing, got %+v", final)
	}
}

func TestAutopilotManager_AdvanceMarksProjectBusyWhileStepping(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 1, 40, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Busy During Advance"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:          "task-1",
				Title:       "Busy check",
				Status:      "todo",
				Assignee:    "dev-1",
				Role:        "developer",
				TestCommand: "go test ./internal/project",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	runner := &blockingAdvanceRunner{
		waitStarted: make(chan struct{}, 1),
		waitGate:    make(chan struct{}),
	}
	manager := NewAutopilotManager(store, runner, func(context.Context) error { return nil }, nil)

	var (
		wg     sync.WaitGroup
		stepErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stepErr = manager.Advance(context.Background(), created.ID)
	}()

	select {
	case <-runner.waitStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected advance runner to start waiting")
	}
	if !manager.IsRunning(created.ID) {
		t.Fatal("expected project to be marked busy while synchronous advance is running")
	}

	close(runner.waitGate)
	wg.Wait()
	if stepErr != nil {
		t.Fatalf("advance project: %v", stepErr)
	}
	if manager.IsRunning(created.ID) {
		t.Fatal("expected busy flag to clear after synchronous advance finishes")
	}
}

func TestAutopilotManager_AdvanceDoesNotSleepOnBlockedStep(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 1, 45, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Blocked Step"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Columns: []string{"todo", "in_progress", "review", "done", "blocked"},
		Tasks: []BoardTask{
			{
				ID:     "task-1",
				Title:  "Nothing actionable",
				Status: "blocked",
			},
		},
	}); err != nil {
		t.Fatalf("seed blocked board: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.cronInterval = time.Second

	start := time.Now()
	current, err := manager.Advance(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("advance blocked project: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("expected blocked advance to return immediately, took %s", elapsed)
	}
	if manager.IsRunning(created.ID) {
		t.Fatalf("expected blocked advance to avoid starting the background loop")
	}
	if current.RunStatus != AutopilotStatusBlocked || current.Status != PhaseStatusBlocked {
		t.Fatalf("expected blocked snapshot after single blocked step, got %+v", current)
	}
}

func TestAutopilotManager_EscalateBlocksCurrentRun(t *testing.T) {
	store := NewStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC)
	})
	created, err := store.Create(CreateInput{Name: "Escalate Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	manager := NewAutopilotManager(store, stagedTaskRunner{}, func(context.Context) error { return nil }, nil)
	manager.setRun(created.ID, func(item *AutopilotRun) {
		item.ProjectID = created.ID
		item.RunID = "autopilot-escalate"
		item.Status = AutopilotStatusRunning
		item.Message = "Dispatching todo tasks"
		item.Iterations = 2
		item.StartedAt = "2026-03-26T02:00:00Z"
	})

	if err := manager.Escalate(created.ID, "Need explicit approval"); err != nil {
		t.Fatalf("escalate project: %v", err)
	}

	run, ok := manager.Status(created.ID)
	if !ok {
		t.Fatal("expected blocked run after escalation")
	}
	if run.Status != AutopilotStatusBlocked {
		t.Fatalf("expected blocked run status, got %+v", run)
	}
	if run.Message != "Need explicit approval" {
		t.Fatalf("expected escalation message to be persisted, got %+v", run)
	}

	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Status != "blocked" || state.Phase != "blocked" {
		t.Fatalf("expected blocked state after escalation, got %+v", state)
	}
}

type blockingAdvanceRunner struct {
	waitStarted chan struct{}
	waitGate    chan struct{}
}

func (r *blockingAdvanceRunner) Start(_ context.Context, req TaskRunRequest) (TaskRun, error) {
	return TaskRun{
		ID:         "run-" + req.TaskID,
		TaskID:     req.TaskID,
		Agent:      req.Agent,
		WorkerKind: req.WorkerKind,
		Status:     TaskRunStatusAccepted,
	}, nil
}

func (r *blockingAdvanceRunner) Wait(_ context.Context, runID string) (TaskRun, error) {
	select {
	case r.waitStarted <- struct{}{}:
	default:
	}
	<-r.waitGate
	return TaskRun{
		ID:         runID,
		TaskID:     "task-1",
		Agent:      "dev-1",
		WorkerKind: WorkerKindCodexCLI,
		Status:     TaskRunStatusCompleted,
		Response: `<task-report>
status: completed
summary: step finished
tests: passed
</task-report>`,
	}, nil
}
