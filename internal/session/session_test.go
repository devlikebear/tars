package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreateAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	s1, err := store.Create("first session")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if s1.Title != "first session" {
		t.Fatalf("expected title 'first session', got %q", s1.Title)
	}
	if s1.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	s2, err := store.Create("second session")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	got, err := store.Get(s1.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != s1.ID || got.Title != s1.Title {
		t.Fatalf("unexpected session: %+v", got)
	}

	_ = s2 // use s2
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	s, err := store.Create("to delete")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Also create a transcript file to verify it gets cleaned up
	tPath := filepath.Join(dir, "sessions", s.ID+".jsonl")
	if err := AppendMessage(tPath, Message{Role: "user", Content: "test"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := store.Delete(s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(sessions))
	}

	_, err = store.Get(s.ID)
	if err == nil {
		t.Fatal("expected error getting deleted session")
	}
}

func TestStoreGetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestStoreTouchAndLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	first, err := store.Create("first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.Create("second")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	now := time.Now().UTC().Add(2 * time.Minute)
	if err := store.Touch(first.ID, now); err != nil {
		t.Fatalf("touch first: %v", err)
	}

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != first.ID {
		t.Fatalf("expected touched session to be latest, got %s", latest.ID)
	}

	if err := store.Touch(second.ID, now.Add(1*time.Minute)); err != nil {
		t.Fatalf("touch second: %v", err)
	}
	latest, err = store.Latest()
	if err != nil {
		t.Fatalf("latest second: %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("expected second session to be latest after touch, got %s", latest.ID)
	}
}

func TestStoreLatestNoSessionsReturnsError(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Latest(); err == nil {
		t.Fatalf("expected error when no sessions exist")
	}
}
