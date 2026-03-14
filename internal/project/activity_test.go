package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreActivityRoundtripNewestFirst(t *testing.T) {
	root := t.TempDir()
	times := []time.Time{
		time.Date(2026, 3, 14, 8, 59, 0, 0, time.UTC),
		time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 14, 9, 1, 0, 0, time.UTC),
	}
	nowIndex := 0
	store := NewStore(root, func() time.Time {
		current := times[nowIndex]
		if nowIndex < len(times)-1 {
			nowIndex++
		}
		return current
	})

	created, err := store.Create(CreateInput{Name: "Dashboard Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	first, err := store.AppendActivity(created.ID, ActivityAppendInput{
		TaskID:  "task-1",
		Source:  "pm",
		Kind:    "assignment",
		Status:  "queued",
		Message: "Assign task-1 to dev-1",
		Meta:    map[string]string{"agent": "dev-1"},
	})
	if err != nil {
		t.Fatalf("append first activity: %v", err)
	}
	if first.ProjectID != created.ID {
		t.Fatalf("expected project id %q, got %q", created.ID, first.ProjectID)
	}

	second, err := store.AppendActivity(created.ID, ActivityAppendInput{
		TaskID:  "task-1",
		Source:  "agent",
		Agent:   "dev-1",
		Kind:    "task_status",
		Status:  "in_progress",
		Message: "Started implementing tests",
	})
	if err != nil {
		t.Fatalf("append second activity: %v", err)
	}
	if second.Agent != "dev-1" {
		t.Fatalf("expected agent dev-1, got %q", second.Agent)
	}

	if _, err := os.Stat(filepath.Join(root, "projects", created.ID, "ACTIVITY.jsonl")); err != nil {
		t.Fatalf("expected ACTIVITY.jsonl to be created: %v", err)
	}

	items, err := store.ListActivity(created.ID, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 activity items, got %d", len(items))
	}
	if items[0].Status != "in_progress" {
		t.Fatalf("expected newest item first, got %+v", items)
	}
	if items[1].Source != "pm" {
		t.Fatalf("expected older item second, got %+v", items)
	}
	if items[0].Timestamp <= items[1].Timestamp {
		t.Fatalf("expected newest timestamp first, got %+v", items)
	}
}
