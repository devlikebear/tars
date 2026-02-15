package cron

import (
	"path/filepath"
	"testing"
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
