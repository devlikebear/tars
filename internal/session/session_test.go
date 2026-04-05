package session

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestStoreEnsureMain_ReusesStableMainSession(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main first: %v", err)
	}
	second, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected stable main session, got %q and %q", first.ID, second.ID)
	}
	if first.Kind != "main" || first.Hidden {
		t.Fatalf("unexpected main session metadata: %+v", first)
	}
}

func TestStoreEnsureMain_FindsMainAfterTitleChange(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	// Simulate auto-title renaming the main session
	if err := store.SetTitle(first.ID, "user's first question here"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	// EnsureMain should still find the same session by kind, not title
	second, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main after rename: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same main session after rename, got %q and %q", first.ID, second.ID)
	}
}

func TestStoreEnsureWorker_HidesWorkerSessionFromDefaultList(t *testing.T) {
	store := NewStore(t.TempDir())
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	if worker.Kind != "worker" || !worker.Hidden {
		t.Fatalf("unexpected worker session metadata: %+v", worker)
	}

	visible, err := store.List()
	if err != nil {
		t.Fatalf("list visible: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected hidden worker excluded from visible list, got %+v", visible)
	}

	all, err := store.ListAll()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 || all[0].ID != worker.ID {
		t.Fatalf("expected hidden worker in full list, got %+v", all)
	}
}

func TestStoreEnsureWorker_ReusesProjectWorkerSession(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker first: %v", err)
	}
	second, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected stable worker session, got %q and %q", first.ID, second.ID)
	}
}

func TestStoreCreate_InitializesArtifactWorkDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	if !reflect.DeepEqual(sess.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected default work_dirs [%q], got %+v", artifactDir, sess.WorkDirs)
	}
	if sess.CurrentDir != artifactDir {
		t.Fatalf("expected current_dir %q, got %q", artifactDir, sess.CurrentDir)
	}
	if _, err := os.Stat(artifactDir); err != nil {
		t.Fatalf("expected artifact dir to exist: %v", err)
	}
}

func TestStoreSetWorkDirs_PreservesMandatoryArtifactDirFirst(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	extraDir := testCanonicalPath(t, filepath.Join(root, "games", "2d-survivors"))
	if err := os.MkdirAll(extraDir, 0o755); err != nil {
		t.Fatalf("mkdir extra dir: %v", err)
	}

	if err := store.SetWorkDirs(sess.ID, []string{extraDir}, extraDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	wantDirs := []string{artifactDir, extraDir}
	if !reflect.DeepEqual(got.WorkDirs, wantDirs) {
		t.Fatalf("expected work_dirs %+v, got %+v", wantDirs, got.WorkDirs)
	}
	if got.CurrentDir != extraDir {
		t.Fatalf("expected current_dir %q, got %q", extraDir, got.CurrentDir)
	}

	if err := store.SetWorkDirs(sess.ID, []string{}, ""); err != nil {
		t.Fatalf("reset work dirs: %v", err)
	}

	got, err = store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session after reset: %v", err)
	}
	if !reflect.DeepEqual(got.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected mandatory artifact dir to remain, got %+v", got.WorkDirs)
	}
	if got.CurrentDir != artifactDir {
		t.Fatalf("expected current_dir to fall back to %q, got %q", artifactDir, got.CurrentDir)
	}
}

func TestStoreGet_MigratesLegacyNestedArtifactDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	legacyDir := filepath.Join(root, "workspace", "artifacts", sess.ID)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "report.md")
	if err := os.WriteFile(legacyFile, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	if _, err := store.Get(sess.ID); err != nil {
		t.Fatalf("get session: %v", err)
	}

	migratedFile := filepath.Join(root, "artifacts", sess.ID, "report.md")
	if _, err := os.Stat(migratedFile); err != nil {
		t.Fatalf("expected migrated file at %s: %v", migratedFile, err)
	}
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed, stat err=%v", err)
	}
}
