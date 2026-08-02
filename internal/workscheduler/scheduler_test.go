package workscheduler

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workstore"
)

func TestSchedulerExecutesDAGAndCompletesWork(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	var mu sync.Mutex
	started := []string{}
	executor := &fakeExecutor{adapter: "fake", execute: func(_ context.Context, execution Execution) (ExecutionResult, error) {
		mu.Lock()
		started = append(started, execution.Claim.Step.IdempotencyKey)
		mu.Unlock()
		return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"ok":true}`)}, nil
	}}
	scheduler := newTestScheduler(t, store, "workspace-dag", executor, 2)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-dag", IdempotencyKey: "dag", SourceID: "flow-dag",
		Title: "DAG", Objective: "execute dependency graph", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{
			{Key: "a", Title: "A", Position: 1, Policy: oneAttemptPolicy()},
			{Key: "b", Title: "B", Position: 2, Policy: oneAttemptPolicy()},
			{Key: "c", Title: "C", Position: 3, DependsOn: []string{"a", "b"}, Policy: oneAttemptPolicy()},
		},
	})
	if err != nil {
		t.Fatalf("submit DAG: %v", err)
	}
	if work.State != workstore.WorkStateRunning {
		t.Fatalf("submitted work state=%s want running", work.State)
	}
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || claimed != 2 {
		t.Fatalf("first scheduler tick claimed=%d err=%v", claimed, err)
	}
	eventually(t, func() bool {
		projection, projectionErr := store.GetWorkProjection(context.Background(), work.WorkspaceID, work.ID)
		return projectionErr == nil && countStepState(projection.Steps, workstore.WorkStateDone) == 2
	})
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || claimed != 1 {
		t.Fatalf("second scheduler tick claimed=%d err=%v", claimed, err)
	}
	projection, err := scheduler.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait DAG: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || countStepState(projection.Steps, workstore.WorkStateDone) != 3 {
		t.Fatalf("completed DAG projection = %+v", projection)
	}
	eventStream, eventErrors := scheduler.Watch(context.Background(), work.ID, 0)
	seenCompleted := false
	for event := range eventStream {
		seenCompleted = seenCompleted || event.Type == workstore.EventTypeStepCompleted
	}
	if watchErr := <-eventErrors; watchErr != nil {
		t.Fatalf("watch completed DAG: %v", watchErr)
	}
	if !seenCompleted {
		t.Fatal("watch stream did not include step completion")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(started) != 3 || started[2] != "c" {
		t.Fatalf("DAG start order=%v want c last", started)
	}
}

func TestSchedulerRetriesThenRequestsReview(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	attempts := 0
	executor := &fakeExecutor{adapter: "fake", execute: func(_ context.Context, _ Execution) (ExecutionResult, error) {
		attempts++
		if attempts == 3 {
			return ExecutionResult{Succeeded: true}, nil
		}
		return ExecutionResult{Succeeded: false, Error: "executor failed", Usage: workstore.StepAttemptUsage{Iterations: 1}}, nil
	}}
	scheduler := newTestScheduler(t, store, "workspace-retry", executor, 1)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-retry", IdempotencyKey: "retry", Title: "Retry", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "retry", Title: "Retry", Policy: workstore.StepSchedulePolicy{
			MaxAttempts: 2, RetryLimit: 1, EscalationState: workstore.WorkStateReview,
		}}},
	})
	if err != nil {
		t.Fatalf("submit retry work: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if claimed, runErr := scheduler.RunOnce(context.Background()); runErr != nil || claimed != 1 {
			t.Fatalf("retry tick %d claimed=%d err=%v", attempt, claimed, runErr)
		}
		want := workstore.WorkStateReady
		if attempt == 2 {
			want = workstore.WorkStateReview
		}
		eventually(t, func() bool {
			projection, projectionErr := store.GetWorkProjection(context.Background(), work.WorkspaceID, work.ID)
			return projectionErr == nil && projection.Steps[0].State == want
		})
	}
	projection, err := scheduler.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait reviewed work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateReview || !projection.Schedules[0].HumanResumeRequired {
		t.Fatalf("reviewed projection = %+v", projection)
	}
	projection, err = scheduler.Resume(context.Background(), work.ID, projection.Steps[0].ID, "operator", "review approved")
	if err != nil {
		t.Fatalf("resume reviewed work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateRunning || projection.Steps[0].State != workstore.WorkStateReady {
		t.Fatalf("resumed projection = %+v", projection)
	}
	if claimed, runErr := scheduler.RunOnce(context.Background()); runErr != nil || claimed != 1 {
		t.Fatalf("resumed tick claimed=%d err=%v", claimed, runErr)
	}
	projection, err = scheduler.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait resumed work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || projection.Attempts[len(projection.Attempts)-1].Number != 3 || projection.Schedules[0].CycleAttemptCount != 1 {
		t.Fatalf("completed resumed projection = %+v", projection)
	}
}

func TestSchedulerCancelStopsActiveExecution(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	started := make(chan struct{})
	executor := &fakeExecutor{adapter: "fake", execute: func(ctx context.Context, _ Execution) (ExecutionResult, error) {
		close(started)
		<-ctx.Done()
		return ExecutionResult{}, ctx.Err()
	}}
	scheduler := newTestScheduler(t, store, "workspace-cancel", executor, 1)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-cancel", IdempotencyKey: "cancel", Title: "Cancel", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "blocked", Title: "Blocked", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatalf("submit cancelled work: %v", err)
	}
	if _, err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("run cancelled work: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	projection, err := scheduler.Cancel(context.Background(), work.ID, "operator", "no longer needed")
	if err != nil {
		t.Fatalf("cancel work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateCancelled || projection.Steps[0].State != workstore.WorkStateCancelled || projection.Attempts[0].Status != workstore.AttemptStatusCancelled {
		t.Fatalf("cancelled projection = %+v", projection)
	}
}

func TestSchedulerRecoversReconnectableClaim(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	seed := newTestScheduler(t, store, "workspace-recover", &fakeExecutor{adapter: "external"}, 1)
	work, err := seed.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-recover", IdempotencyKey: "recover", Title: "Recover", Adapter: "external", ActorID: "planner",
		Steps: []StepSpec{{Key: "external", Title: "External", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatalf("submit recovered work: %v", err)
	}
	if _, err := store.PromoteReadySteps(context.Background(), workstore.PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "seed",
	}); err != nil {
		t.Fatalf("promote recovered step: %v", err)
	}
	claim, err := store.ClaimReadyStep(context.Background(), workstore.ClaimReadyStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, WorkerID: "dead-worker",
		Adapter: "external", LeaseDuration: time.Minute, ActorID: "seed",
	})
	if err != nil {
		t.Fatalf("seed recovered claim: %v", err)
	}
	recoveredAttempt := make(chan string, 1)
	executor := &fakeExecutor{adapter: "external", recover: func(_ context.Context, execution Execution) (ExecutionResult, bool, error) {
		recoveredAttempt <- execution.Claim.Attempt.ID
		return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"reconnected":true}`)}, true, nil
	}}
	recovered := newTestScheduler(t, store, "workspace-recover", executor, 1)
	if count, err := recovered.RecoverOnce(context.Background()); err != nil || count != 1 {
		t.Fatalf("recover active claim count=%d err=%v", count, err)
	}
	select {
	case got := <-recoveredAttempt:
		if got != claim.Attempt.ID {
			t.Fatalf("recovered attempt=%s want=%s", got, claim.Attempt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("recover executor was not called")
	}
	projection, err := recovered.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait recovered work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || projection.Attempts[0].Status != workstore.AttemptStatusSucceeded {
		t.Fatalf("recovered projection = %+v", projection)
	}
}

func TestSchedulerRejectsCyclicSubmissionBeforePersistence(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler := newTestScheduler(t, store, "workspace-cycle", &fakeExecutor{adapter: "fake"}, 1)
	_, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-cycle", IdempotencyKey: "cycle", Title: "Cycle",
		Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{
			{Key: "a", Title: "A", DependsOn: []string{"b"}, Policy: oneAttemptPolicy()},
			{Key: "b", Title: "B", DependsOn: []string{"a"}, Policy: oneAttemptPolicy()},
		},
	})
	if !errors.Is(err, workstore.ErrDependencyCycle) {
		t.Fatalf("cyclic submit error=%v want ErrDependencyCycle", err)
	}
	works, listErr := store.ListWorks(context.Background(), workstore.ListWorksFilter{WorkspaceID: "workspace-cycle"})
	if listErr != nil {
		t.Fatalf("list work after cyclic submit: %v", listErr)
	}
	if len(works) != 0 {
		t.Fatalf("cyclic submit persisted works = %+v", works)
	}
}

type fakeExecutor struct {
	adapter string
	execute func(context.Context, Execution) (ExecutionResult, error)
	recover func(context.Context, Execution) (ExecutionResult, bool, error)
}

func (executor *fakeExecutor) Adapter() string { return executor.adapter }

func (executor *fakeExecutor) Execute(ctx context.Context, execution Execution) (ExecutionResult, error) {
	if executor.execute == nil {
		return ExecutionResult{}, errors.New("unexpected execute")
	}
	return executor.execute(ctx, execution)
}

func (executor *fakeExecutor) Recover(ctx context.Context, execution Execution) (ExecutionResult, bool, error) {
	if executor.recover == nil {
		return ExecutionResult{}, false, nil
	}
	return executor.recover(ctx, execution)
}

func newTestScheduler(t *testing.T, store *workstore.Store, workspaceID string, executor Executor, workers int) *Scheduler {
	t.Helper()
	scheduler, err := New(Options{
		Store: store, WorkspaceID: workspaceID, WorkerID: "scheduler-test", ActorID: "scheduler-test",
		LeaseDuration: time.Minute, HeartbeatInterval: 10 * time.Millisecond,
		PollInterval: 5 * time.Millisecond, MaxWorkers: workers, Executors: []Executor{executor},
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	t.Cleanup(func() { scheduler.Close() })
	return scheduler
}

func openSchedulerTestStore(t *testing.T) *workstore.Store {
	t.Helper()
	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open scheduler store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func oneAttemptPolicy() workstore.StepSchedulePolicy {
	return workstore.StepSchedulePolicy{MaxAttempts: 1, EscalationState: workstore.WorkStateReview}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func countStepState(steps []workstore.Step, state workstore.WorkState) int {
	count := 0
	for _, step := range steps {
		if step.State == state {
			count++
		}
	}
	return count
}
