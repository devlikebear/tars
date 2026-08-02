package workstore

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerPromotesDependenciesAndAllowsOneClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-scheduler", "scheduler-dag")
	first := mustCreateScheduledStep(t, store, work, "first", 1)
	second := mustCreateScheduledStep(t, store, work, "second", 2)
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      second.ID,
		DependsOnID: first.ID,
		ActorID:     "planner",
	}); err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	for _, step := range []Step{first, second} {
		if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
			WorkspaceID: work.WorkspaceID,
			WorkID:      work.ID,
			StepID:      step.ID,
			Policy: StepSchedulePolicy{
				MaxAttempts:     2,
				RetryLimit:      1,
				EscalationState: WorkStateReview,
			},
			ActorID: "scheduler",
		}); err != nil {
			t.Fatalf("configure step %s: %v", step.ID, err)
		}
	}

	promoted, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		ActorID:     "scheduler",
	})
	if err != nil {
		t.Fatalf("promote ready steps: %v", err)
	}
	if len(promoted) != 1 || promoted[0].ID != first.ID || promoted[0].State != WorkStateReady {
		t.Fatalf("first promotion = %+v, want only first ready", promoted)
	}

	claim, err := store.ClaimReadyStep(ctx, ClaimReadyStepInput{
		WorkspaceID:   work.WorkspaceID,
		WorkID:        work.ID,
		WorkerID:      "worker-a",
		Adapter:       "local",
		LeaseDuration: time.Minute,
		ActorID:       "scheduler",
	})
	if err != nil {
		t.Fatalf("claim first step: %v", err)
	}
	if claim.Step.ID != first.ID || claim.Step.State != WorkStateRunning || claim.Attempt.Status != AttemptStatusRunning || claim.Schedule.LeaseOwner != "worker-a" {
		t.Fatalf("claim = %+v", claim)
	}
	if _, err := store.ClaimReadyStep(ctx, ClaimReadyStepInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		WorkerID:    "worker-b",
		Adapter:     "local",
		ActorID:     "scheduler",
	}); !errors.Is(err, ErrNoReadyStep) {
		t.Fatalf("competing claim error = %v, want ErrNoReadyStep", err)
	}
	if _, err := store.HeartbeatStepClaim(ctx, HeartbeatStepClaimInput{
		WorkspaceID:   work.WorkspaceID,
		WorkID:        work.ID,
		StepID:        first.ID,
		AttemptID:     claim.Attempt.ID,
		WorkerID:      "worker-b",
		LeaseDuration: time.Minute,
		ActorID:       "worker-b",
	}); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("foreign heartbeat error = %v, want ErrClaimConflict", err)
	}
	if _, err := store.HeartbeatStepClaim(ctx, HeartbeatStepClaimInput{
		WorkspaceID:   work.WorkspaceID,
		WorkID:        work.ID,
		StepID:        first.ID,
		AttemptID:     claim.Attempt.ID,
		WorkerID:      "worker-a",
		LeaseDuration: 2 * time.Minute,
		ActorID:       "worker-a",
	}); err != nil {
		t.Fatalf("heartbeat claim: %v", err)
	}
	resolution, err := store.CompleteStepAttempt(ctx, CompleteStepAttemptInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      first.ID,
		AttemptID:   claim.Attempt.ID,
		WorkerID:    "worker-a",
		Succeeded:   true,
		ActorID:     "worker-a",
	})
	if err != nil {
		t.Fatalf("complete first step: %v", err)
	}
	if resolution.Step.State != WorkStateDone || resolution.Attempt.Status != AttemptStatusSucceeded || resolution.Disposition != StepDispositionDone {
		t.Fatalf("completion = %+v", resolution)
	}

	promoted, err = store.PromoteReadySteps(ctx, PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		ActorID:     "scheduler",
	})
	if err != nil {
		t.Fatalf("promote dependent step: %v", err)
	}
	if len(promoted) != 1 || promoted[0].ID != second.ID {
		t.Fatalf("dependent promotion = %+v, want second", promoted)
	}

	projection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get scheduler projection: %v", err)
	}
	for _, eventType := range []EventType{
		EventTypeStepReady,
		EventTypeStepClaimed,
		EventTypeStepHeartbeat,
		EventTypeAttemptCompleted,
		EventTypeStepCompleted,
	} {
		if !hasEventType(projection.Events, eventType) {
			t.Fatalf("scheduler events missing %q: %+v", eventType, projection.Events)
		}
	}
}

func TestSchedulerCompetingWorkersCannotShareClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-race", "scheduler-race")
	step := mustCreateScheduledStep(t, store, work, "only", 1)
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      step.ID,
		Policy:      StepSchedulePolicy{MaxAttempts: 1, EscalationState: WorkStateBlocked},
		ActorID:     "scheduler",
	}); err != nil {
		t.Fatalf("configure schedule: %v", err)
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "scheduler"}); err != nil {
		t.Fatalf("promote ready step: %v", err)
	}

	const competitors = 24
	var successes atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < competitors; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			_, claimErr := store.ClaimReadyStep(ctx, ClaimReadyStepInput{
				WorkspaceID:   work.WorkspaceID,
				WorkID:        work.ID,
				WorkerID:      "worker-" + string(rune('a'+worker)),
				Adapter:       "local",
				LeaseDuration: time.Minute,
				ActorID:       "scheduler",
			})
			switch {
			case claimErr == nil:
				successes.Add(1)
			case errors.Is(claimErr, ErrNoReadyStep):
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("competing claims successes=%d unexpected=%d", successes.Load(), unexpected.Load())
	}
	schedule, err := store.GetStepSchedule(ctx, work.WorkspaceID, work.ID, step.ID)
	if err != nil {
		t.Fatalf("get winning schedule: %v", err)
	}
	if schedule.LeaseOwner == "" || schedule.ActiveAttemptID == "" || schedule.AttemptCount != 1 {
		t.Fatalf("winning schedule = %+v", schedule)
	}
}

func TestSchedulerReclaimsAndEscalatesByBoundedPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var nowMu sync.Mutex
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ledger.db"), Options{Now: func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return now
	}})
	if err != nil {
		t.Fatalf("open scheduler store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	advance := func(delta time.Duration) {
		nowMu.Lock()
		now = now.Add(delta)
		nowMu.Unlock()
	}

	work := mustCreateWork(t, store, "workspace-policy", "scheduler-policy")
	step := mustCreateScheduledStep(t, store, work, "bounded", 1)
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      step.ID,
		Policy: StepSchedulePolicy{
			MaxAttempts:     4,
			RetryLimit:      1,
			ReplanLimit:     1,
			DecomposeLimit:  1,
			MaxIterations:   8,
			MaxTokens:       1000,
			MaxCostUSD:      1,
			EscalationState: WorkStateReview,
		},
		ActorID: "scheduler",
	}); err != nil {
		t.Fatalf("configure bounded policy: %v", err)
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "scheduler"}); err != nil {
		t.Fatalf("promote policy step: %v", err)
	}

	claim := mustClaimStep(t, store, work, "worker-1", time.Second)
	advance(2 * time.Second)
	reclaimed, err := store.ReclaimExpiredStepClaims(ctx, ReclaimExpiredStepClaimsInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		ActorID:     "recovery",
		Reason:      "worker lease expired",
	})
	if err != nil {
		t.Fatalf("reclaim expired claim: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].Attempt.ID != claim.Attempt.ID || reclaimed[0].Disposition != StepDispositionRetry || reclaimed[0].Step.State != WorkStateReady {
		t.Fatalf("first reclaim = %+v", reclaimed)
	}

	for attemptNumber, wantDisposition := range []StepDisposition{
		StepDispositionReplan,
		StepDispositionDecompose,
		StepDispositionReview,
	} {
		claim = mustClaimStep(t, store, work, "worker-policy", time.Minute)
		resolution, completeErr := store.CompleteStepAttempt(ctx, CompleteStepAttemptInput{
			WorkspaceID: work.WorkspaceID,
			WorkID:      work.ID,
			StepID:      step.ID,
			AttemptID:   claim.Attempt.ID,
			WorkerID:    "worker-policy",
			Succeeded:   false,
			ErrorText:   "attempt failed",
			Usage: StepAttemptUsage{
				Iterations: 1,
				Tokens:     100,
				CostUSD:    0.1,
			},
			ActorID: "worker-policy",
		})
		if completeErr != nil {
			t.Fatalf("complete failed attempt %d: %v", attemptNumber+2, completeErr)
		}
		if resolution.Disposition != wantDisposition {
			t.Fatalf("attempt %d disposition=%s want=%s", attemptNumber+2, resolution.Disposition, wantDisposition)
		}
	}

	schedule, err := store.GetStepSchedule(ctx, work.WorkspaceID, work.ID, step.ID)
	if err != nil {
		t.Fatalf("get escalated schedule: %v", err)
	}
	if schedule.LastDisposition != StepDispositionReview || schedule.BlockedReason != "attempt failed" || !schedule.HumanResumeRequired {
		t.Fatalf("escalated schedule = %+v", schedule)
	}
	resumed, err := store.ResumeScheduledStep(ctx, ResumeScheduledStepInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      step.ID,
		ActorID:     "operator",
		Reason:      "operator supplied missing context",
	})
	if err != nil {
		t.Fatalf("resume reviewed step: %v", err)
	}
	if resumed.Step.State != WorkStateReady || resumed.Schedule.HumanResumeRequired || resumed.Schedule.NextAction != StepExecutionActionExecute {
		t.Fatalf("resumed step = %+v", resumed)
	}
	claim = mustClaimStep(t, store, work, "worker-resumed", time.Minute)
	if claim.Attempt.Number != 5 {
		t.Fatalf("resumed attempt number=%d want=5", claim.Attempt.Number)
	}
	if claim.Schedule.CycleAttemptCount != 1 {
		t.Fatalf("resumed cycle attempt count=%d want=1", claim.Schedule.CycleAttemptCount)
	}
}

func mustCreateScheduledStep(t *testing.T, store *Store, work Work, key string, position int) Step {
	t.Helper()
	step, err := store.CreateStep(context.Background(), CreateStepInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "scheduler-step:" + key,
		Title:          key,
		State:          WorkStateTodo,
		Position:       position,
		ActorID:        "planner",
	})
	if err != nil {
		t.Fatalf("create scheduled step %s: %v", key, err)
	}
	return step
}

func mustClaimStep(t *testing.T, store *Store, work Work, workerID string, lease time.Duration) StepClaim {
	t.Helper()
	claim, err := store.ClaimReadyStep(context.Background(), ClaimReadyStepInput{
		WorkspaceID:   work.WorkspaceID,
		WorkID:        work.ID,
		WorkerID:      workerID,
		Adapter:       "local",
		LeaseDuration: lease,
		ActorID:       "scheduler",
	})
	if err != nil {
		t.Fatalf("claim scheduled step: %v", err)
	}
	return claim
}

func hasEventType(events []Event, want EventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}
