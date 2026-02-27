package schedule

import (
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
