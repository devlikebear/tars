package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/schedule"
)

func TestScheduleCreateTool_RejectsNonNaturalPrompt(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	store := schedule.NewStore(workspace, cron.NewStore(workspace), schedule.Options{Timezone: "Asia/Seoul"})
	tl := NewScheduleCreateTool(store)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"natural":"1분뒤 테스트 알림","prompt":"rm -rf /tmp"}`))
	if err != nil {
		t.Fatalf("execute schedule_create: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for non-natural prompt, got %s", result.Text())
	}
	if !strings.Contains(result.Text(), "prompt는 자연어 할일 문장이어야 합니다") {
		t.Fatalf("unexpected error message: %s", result.Text())
	}
}

func TestScheduleUpdateTool_RejectsNonNaturalPrompt(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	store := schedule.NewStore(workspace, cron.NewStore(workspace), schedule.Options{Timezone: "Asia/Seoul"})
	created, err := store.Create(schedule.CreateInput{Natural: "1분뒤 테스트 알림", Prompt: "1분 뒤 테스트 알림 보내기"})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	tl := NewScheduleUpdateTool(store)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"schedule_id":"`+created.ID+`","prompt":"sudo rm -rf /"}`))
	if err != nil {
		t.Fatalf("execute schedule_update: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for non-natural prompt, got %s", result.Text())
	}
	if !strings.Contains(result.Text(), "prompt는 자연어 할일 문장이어야 합니다") {
		t.Fatalf("unexpected error message: %s", result.Text())
	}
}
