package cron

import (
	"testing"
	"time"
)

func TestShouldRunAt_CronExpressionUsesJobTimezone(t *testing.T) {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// Ran at 09:00 KST on the 15th; the next run is 09:00 KST on the 16th.
	lastRun := time.Date(2026, 2, 15, 9, 0, 0, 0, seoul)
	job := Job{
		Enabled:   true,
		Schedule:  "0 9 * * *",
		Timezone:  "Asia/Seoul",
		CreatedAt: lastRun.Add(-time.Hour),
		LastRunAt: &lastRun,
	}

	if shouldRunAt(job, time.Date(2026, 2, 16, 8, 59, 0, 0, seoul)) {
		t.Fatalf("job ran before 09:00 in its own timezone")
	}
	if !shouldRunAt(job, time.Date(2026, 2, 16, 9, 0, 0, 0, seoul)) {
		t.Fatalf("job did not run at 09:00 in its own timezone")
	}
	// 09:00 UTC is 18:00 KST; the run belongs to 09:00 KST, so it is long past due.
	if !shouldRunAt(job, time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("job did not run at 00:00 UTC (09:00 KST)")
	}
}

func TestShouldRunAt_LocalTimezoneFollowsServerLocal(t *testing.T) {
	lastRun := time.Date(2026, 2, 15, 9, 0, 0, 0, time.Local)
	job := Job{
		Enabled:   true,
		Schedule:  "0 9 * * *",
		Timezone:  LocalTimezone,
		CreatedAt: lastRun.Add(-time.Hour),
		LastRunAt: &lastRun,
	}

	if shouldRunAt(job, time.Date(2026, 2, 16, 8, 59, 0, 0, time.Local).UTC()) {
		t.Fatalf("job ran before 09:00 local")
	}
	if !shouldRunAt(job, time.Date(2026, 2, 16, 9, 0, 0, 0, time.Local).UTC()) {
		t.Fatalf("job did not run at 09:00 local")
	}
}

func TestShouldRunAt_JobWithoutTimezoneKeepsUTC(t *testing.T) {
	// Jobs saved before the timezone field existed ran in UTC; they keep doing so.
	lastRun := time.Date(2026, 2, 15, 9, 0, 0, 0, time.UTC)
	job := Job{
		Enabled:   true,
		Schedule:  "0 9 * * *",
		CreatedAt: lastRun.Add(-time.Hour),
		LastRunAt: &lastRun,
	}

	if shouldRunAt(job, time.Date(2026, 2, 16, 8, 59, 0, 0, time.UTC)) {
		t.Fatalf("legacy job ran before 09:00 UTC")
	}
	if !shouldRunAt(job, time.Date(2026, 2, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("legacy job did not run at 09:00 UTC")
	}
}

func TestShouldRunAt_CronTZPrefixWinsOverJobTimezone(t *testing.T) {
	lastRun := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC) // 09:00 KST
	job := Job{
		Enabled:   true,
		Schedule:  "CRON_TZ=Asia/Seoul 0 9 * * *",
		Timezone:  "America/New_York",
		CreatedAt: lastRun.Add(-time.Hour),
		LastRunAt: &lastRun,
	}

	if shouldRunAt(job, time.Date(2026, 2, 15, 23, 59, 0, 0, time.UTC)) {
		t.Fatalf("job ran before 09:00 KST")
	}
	if !shouldRunAt(job, time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("job did not run at 09:00 KST")
	}
}

func TestJobLocation(t *testing.T) {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	tests := []struct {
		name     string
		timezone string
		want     *time.Location
	}{
		{name: "empty is legacy utc", timezone: "", want: time.UTC},
		{name: "local", timezone: "Local", want: time.Local},
		{name: "iana name", timezone: " Asia/Seoul ", want: seoul},
		{name: "unknown falls back to local", timezone: "Nowhere/Land", want: time.Local},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Job{Timezone: tc.timezone}.location()
			if got.String() != tc.want.String() {
				t.Fatalf("location(%q) = %s, want %s", tc.timezone, got, tc.want)
			}
		})
	}
}

func TestStore_CreateStampsLocalTimezone(t *testing.T) {
	store := NewStore(t.TempDir())
	job, err := store.CreateWithOptions(CreateInput{Prompt: "brief me", Schedule: "0 9 * * 1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.Timezone != LocalTimezone {
		t.Fatalf("new job timezone = %q, want %q", job.Timezone, LocalTimezone)
	}
	got, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Timezone != LocalTimezone {
		t.Fatalf("stored job timezone = %q, want %q", got.Timezone, LocalTimezone)
	}
}

func TestStore_UpdateScheduleMovesLegacyJobToLocal(t *testing.T) {
	store := NewStore(t.TempDir())
	job, err := store.CreateWithOptions(CreateInput{Prompt: "brief me", Schedule: "0 9 * * 1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate a job saved before the timezone field existed.
	jobs, err := store.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	jobs[0].Timezone = ""
	if err := store.save(jobs); err != nil {
		t.Fatalf("save: %v", err)
	}

	name := "renamed"
	renamed, err := store.Update(job.ID, UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("update name: %v", err)
	}
	if renamed.Timezone != "" {
		t.Fatalf("renaming a legacy job changed its timezone to %q", renamed.Timezone)
	}

	schedule := "0 10 * * 1"
	rescheduled, err := store.Update(job.ID, UpdateInput{Schedule: &schedule})
	if err != nil {
		t.Fatalf("update schedule: %v", err)
	}
	if rescheduled.Timezone != LocalTimezone {
		t.Fatalf("rescheduled legacy job timezone = %q, want %q", rescheduled.Timezone, LocalTimezone)
	}
}
