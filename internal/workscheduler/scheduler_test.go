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

func TestSchedulerRequiresIndependentProofBeforeCompletingStep(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	executor := &fakeExecutor{adapter: "fake", execute: func(_ context.Context, _ Execution) (ExecutionResult, error) {
		return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"worker":"success"}`)}, nil
	}}
	verifier := &fakeVerifier{
		name: "deterministic", id: "verifier-1",
		environment: json.RawMessage(`{"runner":"proof-process","workspace":"read-only-copy"}`),
		verify: func(_ context.Context, request VerificationRequest) (VerificationResult, error) {
			if request.Execution.Claim.Schedule.LeaseOwner != "worker-primary" || request.Requirement.Kind != "test" {
				t.Fatalf("verification request = %+v", request)
			}
			return VerificationResult{
				Status: workstore.ProofStatusPassed, Summary: "tests passed independently",
				Rationale: "exit code 0", SubjectDigest: "sha256:verified-source",
				ArtifactDigestsJSON: json.RawMessage(`["sha256:test-log"]`),
			}, nil
		},
	}
	scheduler, err := New(Options{
		Store: store, WorkspaceID: "workspace-proof", WorkerID: "worker-primary", ActorID: "scheduler",
		LeaseDuration: time.Minute, HeartbeatInterval: 10 * time.Millisecond, PollInterval: 5 * time.Millisecond,
		MaxWorkers: 1, Executors: []Executor{executor}, Verifiers: []Verifier{verifier},
	})
	if err != nil {
		t.Fatalf("new proof scheduler: %v", err)
	}
	t.Cleanup(scheduler.Close)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-proof", IdempotencyKey: "proof", Title: "Proof",
		Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "test", Title: "Test", Policy: workstore.StepSchedulePolicy{
			MaxAttempts: 1, EscalationState: workstore.WorkStateReview,
			Proof: workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview,
				Requirements: []workstore.ProofRequirement{{Kind: "test", Verifier: "deterministic", Command: "go test ./..."}}},
		}}},
	})
	if err != nil {
		t.Fatalf("submit proof-gated work: %v", err)
	}
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || claimed != 1 {
		t.Fatalf("run proof-gated work claimed=%d err=%v", claimed, err)
	}
	projection, err := scheduler.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait proof-gated work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || projection.Steps[0].State != workstore.WorkStateDone {
		t.Fatalf("proof-gated projection = %+v", projection)
	}
	if len(projection.Proofs) != 2 {
		t.Fatalf("proof count=%d want worker report and independent proof", len(projection.Proofs))
	}
	if projection.Proofs[0].Status != workstore.ProofStatusReported || projection.Proofs[0].Origin != workstore.ProofOriginWorkerReport {
		t.Fatalf("worker report = %+v", projection.Proofs[0])
	}
	verified := projection.Proofs[1]
	if verified.Status != workstore.ProofStatusPassed || verified.Origin != workstore.ProofOriginIndependentVerifier || verified.ReporterID != "worker-primary" || verified.VerifierID != "verifier-1" || string(verified.ArtifactDigestsJSON) != `["sha256:test-log"]` {
		t.Fatalf("independent proof = %+v", verified)
	}
}

func TestSchedulerRejectsVerifierWithWorkerIdentity(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	_, err := New(Options{
		Store: store, WorkspaceID: "workspace-proof-identity", WorkerID: "same-id", ActorID: "scheduler",
		Executors: []Executor{&fakeExecutor{adapter: "fake"}},
		Verifiers: []Verifier{&fakeVerifier{name: "deterministic", id: "same-id", environment: json.RawMessage(`{"runner":"same"}`)}},
	})
	if err == nil {
		t.Fatal("scheduler accepted a verifier with the worker identity")
	}
}

func TestHeartbeatFailureStopsOnlyAfterAuthorityIsLost(t *testing.T) {
	t.Parallel()

	leaseExpiresAt := time.Now().Add(time.Minute)
	if shouldStopHeartbeat(context.DeadlineExceeded, leaseExpiresAt, time.Now()) {
		t.Fatal("transient heartbeat timeout stopped a still-valid claim")
	}
	if !shouldStopHeartbeat(workstore.ErrClaimConflict, leaseExpiresAt, time.Now()) {
		t.Fatal("claim conflict did not stop heartbeat")
	}
	if !shouldStopHeartbeat(context.DeadlineExceeded, time.Now().Add(-time.Millisecond), time.Now()) {
		t.Fatal("heartbeat continued after the last known lease expired")
	}
}

func TestSchedulerWorkerSuccessCannotOverrideFailedIndependentProof(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler, err := New(Options{
		Store: store, WorkspaceID: "workspace-proof-failed", WorkerID: "worker-primary", ActorID: "scheduler",
		LeaseDuration: time.Minute, HeartbeatInterval: 10 * time.Millisecond, PollInterval: 5 * time.Millisecond,
		MaxWorkers: 1,
		Executors: []Executor{&fakeExecutor{adapter: "fake", execute: func(_ context.Context, _ Execution) (ExecutionResult, error) {
			return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"worker":"success"}`)}, nil
		}}},
		Verifiers: []Verifier{&fakeVerifier{
			name: "deterministic", id: "verifier-1",
			environment: json.RawMessage(`{"runner":"proof-process"}`),
			verify: func(_ context.Context, _ VerificationRequest) (VerificationResult, error) {
				return VerificationResult{
					Status: workstore.ProofStatusFailed, Summary: "tests failed",
					Rationale: "exit code 1", SubjectDigest: "sha256:failed-source",
				}, nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("new failed-proof scheduler: %v", err)
	}
	t.Cleanup(scheduler.Close)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		WorkspaceID: "workspace-proof-failed", IdempotencyKey: "proof-failed", Title: "Proof failed",
		Adapter: "fake", ActorID: "planner", Steps: []StepSpec{{Key: "test", Title: "Test", Policy: workstore.StepSchedulePolicy{
			MaxAttempts: 1, EscalationState: workstore.WorkStateReview,
			Proof: workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview,
				Requirements: []workstore.ProofRequirement{{Kind: "test", Verifier: "deterministic"}}},
		}}},
	})
	if err != nil {
		t.Fatalf("submit failed-proof work: %v", err)
	}
	if _, err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("run failed-proof work: %v", err)
	}
	projection, err := scheduler.Wait(context.Background(), work.ID)
	if err != nil {
		t.Fatalf("wait failed-proof work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateReview || projection.Steps[0].State != workstore.WorkStateReview || projection.Attempts[0].Status != workstore.AttemptStatusSucceeded || projection.Proofs[1].Status != workstore.ProofStatusFailed {
		t.Fatalf("failed-proof projection = %+v", projection)
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

type fakeVerifier struct {
	name        string
	id          string
	environment json.RawMessage
	verify      func(context.Context, VerificationRequest) (VerificationResult, error)
}

func (verifier *fakeVerifier) Name() string { return verifier.name }

func (verifier *fakeVerifier) Identity() VerifierIdentity {
	return VerifierIdentity{ID: verifier.id, EnvironmentJSON: verifier.environment}
}

func (verifier *fakeVerifier) Verify(ctx context.Context, request VerificationRequest) (VerificationResult, error) {
	if verifier.verify == nil {
		return VerificationResult{}, errors.New("unexpected verification")
	}
	return verifier.verify(ctx, request)
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
