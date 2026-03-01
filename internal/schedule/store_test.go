package schedule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
)

func TestStore_CreateNaturalScheduleAndComplete(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	cronStore := cron.NewStore(workspace)
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.FixedZone("KST", 9*3600))
	store := NewStore(workspace, cronStore, Options{Now: func() time.Time { return now }, Timezone: "Asia/Seoul"})

	item, err := store.Create(CreateInput{Natural: "내일 오후 3시에 회의 준비 알려줘"})
	if err != nil {
		t.Fatalf("create natural schedule: %v", err)
	}
	if !strings.HasPrefix(item.Schedule, "at:") {
		t.Fatalf("expected at: schedule, got %q", item.Schedule)
	}
	if item.CronJobID == "" {
		t.Fatalf("expected cron job id")
	}

	completed, err := store.Complete(item.ID)
	if err != nil {
		t.Fatalf("complete schedule: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}
	job, err := cronStore.Get(item.CronJobID)
	if err != nil {
		t.Fatalf("get cron job: %v", err)
	}
	if job.Enabled {
		t.Fatalf("expected cron job disabled after complete")
	}
}

func TestStore_CreateWeeklyNaturalSchedule(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	cronStore := cron.NewStore(workspace)
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.FixedZone("KST", 9*3600))
	store := NewStore(workspace, cronStore, Options{Now: func() time.Time { return now }, Timezone: "Asia/Seoul"})

	item, err := store.Create(CreateInput{Natural: "매주 월요일 9시 팀 동기화"})
	if err != nil {
		t.Fatalf("create weekly schedule: %v", err)
	}
	if item.Schedule != "0 9 * * 1" {
		t.Fatalf("expected weekly cron expression, got %q", item.Schedule)
	}
}

func TestStore_UsesCronAsSingleStorage(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	cronStore := cron.NewStore(workspace)
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.FixedZone("KST", 9*3600))
	store := NewStore(workspace, cronStore, Options{Now: func() time.Time { return now }, Timezone: "Asia/Seoul"})

	item, err := store.Create(CreateInput{Natural: "내일 오후 3시에 회의 준비 알려줘"})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if item.ID == "" || item.CronJobID == "" {
		t.Fatalf("expected schedule id and cron id")
	}
	if item.ID != item.CronJobID {
		t.Fatalf("expected schedule id == cron id, got schedule=%q cron=%q", item.ID, item.CronJobID)
	}
	if _, err := os.Stat(filepath.Join(workspace, "schedule", "items.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("expected no legacy schedule items file, err=%v", err)
	}
}

func TestStore_MigrateLegacyItemsToCron(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	scheduleDir := filepath.Join(workspace, "schedule")
	if err := os.MkdirAll(scheduleDir, 0o755); err != nil {
		t.Fatalf("mkdir schedule dir: %v", err)
	}
	itemsPath := filepath.Join(scheduleDir, "items.jsonl")
	legacy := legacyRecord{
		Op: "upsert",
		Item: Item{
			ID:        "sch_legacy",
			Title:     "legacy title",
			Prompt:    "legacy prompt",
			Natural:   "내일 오후 3시에 레거시 테스트",
			Schedule:  "at:2026-03-01T06:00:00Z",
			Status:    "completed",
			ProjectID: "proj_legacy",
			Timezone:  "Asia/Seoul",
			CreatedAt: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC),
		},
		Timestamp: time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC),
	}
	line, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy record: %v", err)
	}
	if err := os.WriteFile(itemsPath, append(line, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy items: %v", err)
	}

	cronStore := cron.NewStore(workspace)
	store := NewStore(workspace, cronStore, Options{Timezone: "Asia/Seoul"})
	items, err := store.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one migrated item, got %d", len(items))
	}
	item := items[0]
	if item.Title != "legacy title" {
		t.Fatalf("expected migrated title, got %q", item.Title)
	}
	if item.Status != "completed" {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	job, err := cronStore.Get(item.ID)
	if err != nil {
		t.Fatalf("get migrated cron job: %v", err)
	}
	if job.Enabled {
		t.Fatalf("expected migrated cron job disabled for completed schedule")
	}
	if _, err := os.Stat(itemsPath + ".migrated.bak"); err != nil {
		t.Fatalf("expected migrated backup file, err=%v", err)
	}
}
