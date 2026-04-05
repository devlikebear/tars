package reflection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

type fakeSessionDeleter struct {
	dir      string
	sessions []session.Session
	deleted  []string
	listErr  error
	delErr   error
}

func (f *fakeSessionDeleter) ListAll() ([]session.Session, error) {
	return f.sessions, f.listErr
}
func (f *fakeSessionDeleter) TranscriptPath(id string) string {
	return filepath.Join(f.dir, id+".jsonl")
}
func (f *fakeSessionDeleter) Delete(id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func touchEmptyTranscript(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestKBCleanupRemovesEmptyOldSession(t *testing.T) {
	dir := t.TempDir()
	touchEmptyTranscript(t, dir, "empty1")
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "empty1", Kind: "chat", UpdatedAt: now.Add(-48 * time.Hour)},
		},
	}
	job := &KBCleanupJob{Sessions: src, EmptySessionAge: 24 * time.Hour, Now: func() time.Time { return now }}
	r, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !r.Success || !r.Changed {
		t.Fatalf("result = %+v", r)
	}
	if got := r.Details["removed_count"].(int); got != 1 {
		t.Errorf("removed = %d, want 1", got)
	}
	if len(src.deleted) != 1 || src.deleted[0] != "empty1" {
		t.Errorf("deleted = %v", src.deleted)
	}
}

func TestKBCleanupSkipsFreshEmptySession(t *testing.T) {
	dir := t.TempDir()
	touchEmptyTranscript(t, dir, "fresh")
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "fresh", Kind: "chat", UpdatedAt: now.Add(-5 * time.Minute)},
		},
	}
	job := &KBCleanupJob{Sessions: src, EmptySessionAge: 24 * time.Hour, Now: func() time.Time { return now }}
	r, _ := job.Run(context.Background())
	if r.Changed || len(src.deleted) > 0 {
		t.Errorf("should not delete fresh session: %+v", r)
	}
}

func TestKBCleanupSkipsMainSession(t *testing.T) {
	dir := t.TempDir()
	touchEmptyTranscript(t, dir, "main")
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "main", Kind: "main", UpdatedAt: now.Add(-100 * 24 * time.Hour)},
		},
	}
	job := &KBCleanupJob{Sessions: src, Now: func() time.Time { return now }}
	r, _ := job.Run(context.Background())
	if r.Changed || len(src.deleted) > 0 {
		t.Errorf("main session must never be deleted: %+v", r)
	}
}

func TestKBCleanupSkipsNonEmptySession(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, "has-content", []session.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	})
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "has-content", Kind: "chat", UpdatedAt: now.Add(-48 * time.Hour)},
		},
	}
	job := &KBCleanupJob{Sessions: src, Now: func() time.Time { return now }}
	r, _ := job.Run(context.Background())
	if r.Changed || len(src.deleted) > 0 {
		t.Errorf("non-empty session must not be deleted: %+v", r)
	}
}

func TestKBCleanupMixed(t *testing.T) {
	dir := t.TempDir()
	touchEmptyTranscript(t, dir, "old-empty")
	touchEmptyTranscript(t, dir, "fresh-empty")
	writeTranscript(t, dir, "old-full", []session.Message{{Role: "user", Content: "hi"}})
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "old-empty", Kind: "chat", UpdatedAt: now.Add(-48 * time.Hour)},
			{ID: "fresh-empty", Kind: "chat", UpdatedAt: now.Add(-1 * time.Hour)},
			{ID: "old-full", Kind: "chat", UpdatedAt: now.Add(-48 * time.Hour)},
		},
	}
	job := &KBCleanupJob{Sessions: src, EmptySessionAge: 24 * time.Hour, Now: func() time.Time { return now }}
	r, _ := job.Run(context.Background())
	if r.Details["removed_count"].(int) != 1 || r.Details["skipped_count"].(int) != 2 {
		t.Errorf("mixed result wrong: %+v", r.Details)
	}
	if len(src.deleted) != 1 || src.deleted[0] != "old-empty" {
		t.Errorf("wrong deletion: %v", src.deleted)
	}
}

func TestKBCleanupNilSource(t *testing.T) {
	job := &KBCleanupJob{}
	r, _ := job.Run(context.Background())
	if r.Success {
		t.Error("nil source should fail")
	}
}

func TestKBCleanupListError(t *testing.T) {
	job := &KBCleanupJob{Sessions: &fakeSessionDeleter{listErr: errors.New("io")}}
	_, err := job.Run(context.Background())
	if err == nil {
		t.Error("expected list error")
	}
}

func TestKBCleanupDeleteErrorRecorded(t *testing.T) {
	dir := t.TempDir()
	touchEmptyTranscript(t, dir, "target")
	now := time.Now()
	src := &fakeSessionDeleter{
		dir: dir,
		sessions: []session.Session{
			{ID: "target", Kind: "chat", UpdatedAt: now.Add(-48 * time.Hour)},
		},
		delErr: errors.New("permission denied"),
	}
	job := &KBCleanupJob{Sessions: src, Now: func() time.Time { return now }}
	r, _ := job.Run(context.Background())
	if r.Changed {
		t.Error("failed delete should not mark as changed")
	}
	errs, ok := r.Details["errors"].([]string)
	if !ok || len(errs) != 1 {
		t.Errorf("errors not recorded: %+v", r.Details)
	}
}
