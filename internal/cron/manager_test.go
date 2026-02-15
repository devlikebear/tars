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
	mgr := NewManager(store, func(_ context.Context, prompt string) (string, error) {
		calls = append(calls, prompt)
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
	if _, err := store.MarkRunResult(notDue.ID, time.Date(2026, 2, 15, 19, 0, 0, 0, time.UTC), nil); err != nil {
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
	mgr := NewManager(store, func(_ context.Context, _ string) (string, error) {
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

	mgr := NewManager(store, func(_ context.Context, _ string) (string, error) {
		return "ok", nil
	}, 100*time.Millisecond, time.Now)
	if err := mgr.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if _, err := store.Get(job.ID); err == nil {
		t.Fatalf("expected delete_after_run job to be removed")
	}
}
