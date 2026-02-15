package cron

import (
	"context"
	"testing"
	"time"
)

func TestManager_TickRunsDueJob(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:      "due-job",
		Prompt:    "run due",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	calls := make([]string, 0, 1)
	mgr := NewManager(store, func(_ context.Context, job Job) (string, error) {
		calls = append(calls, job.Prompt)
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time {
		return time.Date(2026, 2, 15, 19, 0, 0, 0, time.UTC)
	})

	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(calls) != 1 || calls[0] != "run due" {
		t.Fatalf("expected due prompt run once, got %+v", calls)
	}

	reloaded, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("get updated job: %v", err)
	}
	if reloaded.LastRunAt == nil {
		t.Fatalf("expected last_run_at to be recorded")
	}
}

func TestManager_TickSkipsNotDueAndDisabledJobs(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	notDue, err := store.CreateWithOptions(CreateInput{
		Name:      "not-due",
		Prompt:    "skip me",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create not-due: %v", err)
	}
	if _, err := store.MarkRunResult(notDue.ID, time.Date(2026, 2, 15, 19, 0, 0, 0, time.UTC), "ok", nil); err != nil {
		t.Fatalf("mark run result: %v", err)
	}

	if _, err := store.CreateWithOptions(CreateInput{
		Name:      "disabled",
		Prompt:    "disabled",
		Schedule:  "every:1s",
		Enabled:   false,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("create disabled: %v", err)
	}

	runCount := 0
	mgr := NewManager(store, func(_ context.Context, _ Job) (string, error) {
		runCount++
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time {
		return time.Date(2026, 2, 15, 19, 0, 10, 0, time.UTC)
	})

	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if runCount != 0 {
		t.Fatalf("expected no run for not-due/disabled jobs, got %d", runCount)
	}
}

func TestManager_TickDeletesDeleteAfterRunJob(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:           "oneshot",
		Prompt:         "run once",
		Schedule:       "every:1s",
		Enabled:        true,
		HasEnable:      true,
		DeleteAfterRun: true,
	})
	if err != nil {
		t.Fatalf("create oneshot: %v", err)
	}

	mgr := NewManager(store, func(_ context.Context, _ Job) (string, error) {
		return "ok", nil
	}, 100*time.Millisecond, time.Now)
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if _, err := store.Get(job.ID); err == nil {
		t.Fatalf("expected delete_after_run job to be removed")
	}
}

func TestManager_TickRunsCronExpressionAtBoundary(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:      "boundary",
		Prompt:    "run at minute boundary",
		Schedule:  "0 0 * * *",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create cron expression job: %v", err)
	}

	base := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	updated, err := store.MarkRunResult(job.ID, base.Add(-24*time.Hour), "ok", nil)
	if err != nil {
		t.Fatalf("update last run: %v", err)
	}
	if updated.LastRunAt == nil {
		t.Fatalf("expected last_run_at set")
	}

	runCount := 0
	mgr := NewManager(store, func(_ context.Context, _ Job) (string, error) {
		runCount++
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time { return base })
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick at boundary: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected 1 run at cron boundary, got %d", runCount)
	}
}

func TestManager_TickRespectsCronTimezonePrefix(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:      "tz-job",
		Prompt:    "run at seoul 09:00",
		Schedule:  "CRON_TZ=Asia/Seoul 0 9 * * *",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create timezone cron job: %v", err)
	}

	// 09:00 Asia/Seoul == 00:00 UTC
	_, err = store.MarkRunResult(job.ID, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), "ok", nil)
	if err != nil {
		t.Fatalf("update last run: %v", err)
	}

	runCount := 0
	mgr := NewManager(store, func(_ context.Context, _ Job) (string, error) {
		runCount++
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time {
		return time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	})
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick timezone boundary: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected timezone cron job to run, got %d", runCount)
	}
}

func TestManager_TickAppliesFailureBackoff(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:      "failing",
		Prompt:    "fail",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	failNow := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	mgr := NewManager(store, func(_ context.Context, _ Job) (string, error) {
		return "", managerErr("boom")
	}, 100*time.Millisecond, func() time.Time { return failNow })

	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick fail: %v", err)
	}
	afterFail, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("get job after fail: %v", err)
	}
	if afterFail.ConsecutiveFailures != 1 {
		t.Fatalf("expected failure count 1, got %d", afterFail.ConsecutiveFailures)
	}
	if afterFail.BackoffUntil == nil {
		t.Fatalf("expected backoff_until set after failure")
	}

	runCount := 0
	mgr = NewManager(store, func(_ context.Context, _ Job) (string, error) {
		runCount++
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time { return failNow.Add(10 * time.Second) })
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick during backoff: %v", err)
	}
	if runCount != 0 {
		t.Fatalf("expected job skipped during backoff, got runCount=%d", runCount)
	}
}

func TestManager_TickRunsAtScheduleOnlyOnce(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Name:      "once",
		Prompt:    "run once",
		Schedule:  "at:2026-02-16T10:00:00Z",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create at job: %v", err)
	}

	runCount := 0
	mgr := NewManager(store, func(_ context.Context, j Job) (string, error) {
		if j.ID != job.ID {
			t.Fatalf("unexpected job id %s", j.ID)
		}
		runCount++
		return "ok", nil
	}, 100*time.Millisecond, func() time.Time {
		return time.Date(2026, 2, 16, 10, 0, 1, 0, time.UTC)
	})
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick at due time: %v", err)
	}
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick after run: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected at schedule job to run once, got %d", runCount)
	}
}

type managerErr string

func (e managerErr) Error() string { return string(e) }
