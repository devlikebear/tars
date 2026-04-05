package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore_CreateAndList(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.Create("daily check", "summarize unread emails")
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.ID == "" {
		t.Fatalf("expected non-empty job id")
	}
	if !job.Enabled {
		t.Fatalf("expected new job enabled=true")
	}

	jobs, err := store.List()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != job.ID {
		t.Fatalf("expected listed job id %q, got %q", job.ID, jobs[0].ID)
	}
}

func TestStore_PersistsJobsOnDisk(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	created, err := store.Create("nightly", "scan project status")
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	reloaded := NewStore(root)
	jobs, err := reloaded.List()
	if err != nil {
		t.Fatalf("list jobs after reload: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job after reload, got %d", len(jobs))
	}
	if jobs[0].ID != created.ID {
		t.Fatalf("expected persisted id %q, got %q", created.ID, jobs[0].ID)
	}

	if _, err := reloaded.Get(created.ID); err != nil {
		t.Fatalf("get persisted job: %v", err)
	}
}

func TestStore_GetMissingReturnsError(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspace"))

	if _, err := store.Get("missing"); err == nil {
		t.Fatalf("expected not found error for missing job")
	}
}

func TestStore_UpdateAndDelete(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	created, err := store.CreateWithOptions(CreateInput{
		Name:      "daily",
		Prompt:    "check inbox",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	updated, err := store.Update(created.ID, UpdateInput{
		Name:           ptrString("daily-updated"),
		Prompt:         ptrString("check inbox and calendar"),
		Schedule:       ptrString("every:30m"),
		Enabled:        ptrBool(false),
		DeleteAfterRun: ptrBool(true),
	})
	if err != nil {
		t.Fatalf("update job: %v", err)
	}
	if updated.Name != "daily-updated" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if updated.Prompt != "check inbox and calendar" {
		t.Fatalf("expected updated prompt, got %q", updated.Prompt)
	}
	if updated.Schedule != "every:30m" {
		t.Fatalf("expected updated schedule, got %q", updated.Schedule)
	}
	if updated.Enabled {
		t.Fatalf("expected enabled=false after update")
	}
	if !updated.DeleteAfterRun {
		t.Fatalf("expected delete_after_run=true after update")
	}

	if err := store.Delete(created.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	if _, err := store.Get(created.ID); err == nil {
		t.Fatalf("expected not found after delete")
	}
}

func ptrString(v string) *string { return &v }
func ptrBool(v bool) *bool       { return &v }

func TestStore_Create_ValidatesSchedule(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	if _, err := store.CreateWithOptions(CreateInput{
		Prompt:    "invalid schedule job",
		Schedule:  "every:",
		Enabled:   true,
		HasEnable: true,
	}); err == nil {
		t.Fatalf("expected schedule validation error")
	}

	if _, err := store.CreateWithOptions(CreateInput{
		Prompt:    "cron expression job",
		Schedule:  "*/5 * * * *",
		Enabled:   true,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("expected valid cron expression, got %v", err)
	}

	created, err := store.CreateWithOptions(CreateInput{
		Prompt:    "at schedule job",
		Schedule:  " at:2026-03-01T15:00:00+09:00 ",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("expected valid at schedule, got %v", err)
	}
	if want := "at:2026-03-01T06:00:00Z"; created.Schedule != want {
		t.Fatalf("expected normalized at schedule %q, got %q", want, created.Schedule)
	}

	naturalCreated, err := store.CreateWithOptions(CreateInput{
		Prompt:    "english natural schedule job",
		Schedule:  "in 1 minute",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("expected english natural schedule to parse, got %v", err)
	}
	if !strings.HasPrefix(naturalCreated.Schedule, "at:") {
		t.Fatalf("expected natural schedule normalized to at:, got %q", naturalCreated.Schedule)
	}
}

func TestStore_ListRunsReturnsLatestFirst(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Prompt:    "run and record",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	t1 := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	if _, err := store.MarkRunResult(job.ID, t1, "ok-1", nil); err != nil {
		t.Fatalf("mark run t1: %v", err)
	}
	if _, err := store.MarkRunResult(job.ID, t2, "", assertErr("boom")); err != nil {
		t.Fatalf("mark run t2: %v", err)
	}

	runs, err := store.ListRuns(job.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if !runs[0].RanAt.Equal(t2) {
		t.Fatalf("expected latest run first, got %s", runs[0].RanAt)
	}
	if runs[0].Error != "boom" {
		t.Fatalf("expected error message on latest run, got %q", runs[0].Error)
	}
	if runs[1].Response != "ok-1" {
		t.Fatalf("expected response persisted in history, got %q", runs[1].Response)
	}
}

func TestStore_CreateWithSessionWakeDeliveryPayload(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:          "typed",
		Prompt:        "collect updates",
		Schedule:      "at:2026-02-17T09:00:00Z",
		Enabled:       true,
		HasEnable:     true,
		SessionTarget: "main",
		WakeMode:      "agent_loop",
		DeliveryMode:  "session",
		Payload:       json.RawMessage(`{"priority":"high"}`),
	})
	if err != nil {
		t.Fatalf("create typed cron job: %v", err)
	}
	if job.SessionTarget != "main" {
		t.Fatalf("expected session_target main, got %q", job.SessionTarget)
	}
	if job.WakeMode != "agent_loop" {
		t.Fatalf("expected wake_mode agent_loop, got %q", job.WakeMode)
	}
	if job.DeliveryMode != "session" {
		t.Fatalf("expected delivery_mode session, got %q", job.DeliveryMode)
	}
	if string(job.Payload) != `{"priority":"high"}` {
		t.Fatalf("expected payload persisted, got %s", string(job.Payload))
	}
	if job.Schedule != "at:2026-02-17T09:00:00Z" {
		t.Fatalf("expected at schedule normalization, got %q", job.Schedule)
	}
	if !job.DeleteAfterRun {
		t.Fatalf("expected at schedule to default delete_after_run=true")
	}
}

func TestStore_CreateWithSessionBindingDefaultsToSessionDelivery(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:      "monitor",
		Prompt:    "check dashboard",
		Schedule:  "every:10m",
		Enabled:   true,
		HasEnable: true,
		SessionID: "sess-monitor",
	})
	if err != nil {
		t.Fatalf("create session-bound cron job: %v", err)
	}
	if job.SessionID != "sess-monitor" {
		t.Fatalf("expected session_id persisted, got %q", job.SessionID)
	}
	if job.DeliveryMode != "session" {
		t.Fatalf("expected session-bound job to default delivery_mode=session, got %q", job.DeliveryMode)
	}
}

func TestStore_Create_NormalizesCurrentSessionTargetToMain(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:          "typed",
		Prompt:        "collect updates",
		Schedule:      "every:30m",
		Enabled:       true,
		HasEnable:     true,
		SessionTarget: "current",
	})
	if err != nil {
		t.Fatalf("create typed cron job: %v", err)
	}
	if job.SessionTarget != "main" {
		t.Fatalf("expected session_target main, got %q", job.SessionTarget)
	}
}

func TestStore_Create_DefaultDeleteAfterRunForOneShotCron(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:      "once",
		Prompt:    "run once",
		Schedule:  "25 22 15 2 *",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create one-shot cron style job: %v", err)
	}
	if !job.DeleteAfterRun {
		t.Fatalf("expected one-shot cron style to default delete_after_run=true")
	}
}

func TestStore_Create_ExplicitDeleteAfterRunFalseOverridesDefault(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:              "yearly",
		Prompt:            "annual run",
		Schedule:          "25 22 15 2 *",
		Enabled:           true,
		HasEnable:         true,
		DeleteAfterRun:    false,
		HasDeleteAfterRun: true,
	})
	if err != nil {
		t.Fatalf("create explicit false delete_after_run job: %v", err)
	}
	if job.DeleteAfterRun {
		t.Fatalf("expected explicit delete_after_run=false to override default")
	}
}

func TestStore_Create_EveryScheduleKeepsDeleteAfterRunFalseByDefault(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	job, err := store.CreateWithOptions(CreateInput{
		Name:      "loop",
		Prompt:    "repeat",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create repeating job: %v", err)
	}
	if job.DeleteAfterRun {
		t.Fatalf("expected repeating schedule to keep delete_after_run=false by default")
	}
}

func TestStore_RunHistoryPrunesPerJobLimit(t *testing.T) {
	root := t.TempDir()
	store := NewStoreWithOptions(root, StoreOptions{RunHistoryLimit: 2})

	job, err := store.CreateWithOptions(CreateInput{
		Prompt:    "prune history",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	base := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := store.MarkRunResult(job.ID, base.Add(time.Duration(i)*time.Minute), "ok", nil); err != nil {
			t.Fatalf("mark run %d: %v", i, err)
		}
	}

	runs, err := store.ListRuns(job.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected pruned run history length 2, got %d", len(runs))
	}

	runPath := filepath.Join(root, "cron", "runs", job.ID+".jsonl")
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("expected per-job run history file, got %v", err)
	}
}

func TestStore_TryStartRunGuardsConcurrentExecution(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job, err := store.CreateWithOptions(CreateInput{
		Prompt:    "lock",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if ok := store.TryStartRun(job.ID); !ok {
		t.Fatalf("expected first run lock to succeed")
	}
	if ok := store.TryStartRun(job.ID); ok {
		t.Fatalf("expected second run lock to fail while running")
	}
	store.FinishRun(job.ID)
	if ok := store.TryStartRun(job.ID); !ok {
		t.Fatalf("expected run lock to succeed after release")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
