package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndSearchExperiences(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	base := time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC)

	if err := AppendExperience(root, Experience{
		Timestamp:     base,
		Category:      "preference",
		Summary:       "User prefers concise Korean responses",
		Tags:          []string{"user", "language"},
		SourceSession: "sess-1",
		Importance:    7,
		ProjectID:     "proj_demo",
	}); err != nil {
		t.Fatalf("append first experience: %v", err)
	}
	if err := AppendExperience(root, Experience{
		Timestamp:     base.Add(2 * time.Minute),
		Category:      "task_completed",
		Summary:       "Completed API smoke verification for gateway reports",
		Tags:          []string{"gateway", "smoke"},
		SourceSession: "sess-2",
		Importance:    9,
		ProjectID:     "proj_demo",
	}); err != nil {
		t.Fatalf("append second experience: %v", err)
	}

	path := filepath.Join(root, "memory", "experiences.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected experiences jsonl to exist: %v", err)
	}

	hits, err := SearchExperiences(root, SearchOptions{Query: "gateway", Limit: 5})
	if err != nil {
		t.Fatalf("search experiences: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].Category != "task_completed" {
		t.Fatalf("expected task_completed, got %q", hits[0].Category)
	}

	recent, err := SearchExperiences(root, SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("load recent experiences: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected one recent record, got %d", len(recent))
	}
	if recent[0].Summary != "Completed API smoke verification for gateway reports" {
		t.Fatalf("expected latest summary first, got %q", recent[0].Summary)
	}
}
