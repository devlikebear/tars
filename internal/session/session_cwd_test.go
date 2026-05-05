package session

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreEligibleCwds_IncludesArtifactAndWorkDirs(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	extra := testCanonicalPath(t, filepath.Join(root, "projects", "alpha"))
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir extra: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, extra); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	got, err := store.EligibleCwds(sess.ID)
	if err != nil {
		t.Fatalf("eligible cwds: %v", err)
	}
	artifact := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	want := []string{artifact, extra}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("eligible cwds mismatch: got=%v want=%v", got, want)
	}
}

func TestStoreEligibleCwds_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.EligibleCwds("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestStoreGetCurrentDir_FallsBackToArtifact(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cur, err := store.GetCurrentDir(sess.ID)
	if err != nil {
		t.Fatalf("get current dir: %v", err)
	}
	artifact := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	if cur != artifact {
		t.Fatalf("expected fallback to artifact %q, got %q", artifact, cur)
	}
}

func TestStoreGetCurrentDir_ReturnsExplicit(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	extra := testCanonicalPath(t, filepath.Join(root, "projects", "beta"))
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir extra: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, extra); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	cur, err := store.GetCurrentDir(sess.ID)
	if err != nil {
		t.Fatalf("get current dir: %v", err)
	}
	if cur != extra {
		t.Fatalf("expected explicit current %q, got %q", extra, cur)
	}
}

func TestStoreSetCurrentDir_RejectsNonEligibleWithSentinel(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// not in work_dirs
	stranger := testCanonicalPath(t, filepath.Join(root, "elsewhere"))
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatalf("mkdir stranger: %v", err)
	}

	err = store.SetCurrentDir(sess.ID, stranger)
	if err == nil {
		t.Fatal("expected error when setting cwd outside work_dirs")
	}
	if !errors.Is(err, ErrCwdNotEligible) {
		t.Fatalf("expected ErrCwdNotEligible, got %v", err)
	}
}

func TestStoreSetCurrentDir_AcceptsEligible(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	extra := testCanonicalPath(t, filepath.Join(root, "projects", "gamma"))
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir extra: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, ""); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	if err := store.SetCurrentDir(sess.ID, extra); err != nil {
		t.Fatalf("set cwd: %v", err)
	}

	got, err := store.GetCurrentDir(sess.ID)
	if err != nil {
		t.Fatalf("get current dir: %v", err)
	}
	if got != extra {
		t.Fatalf("expected current %q, got %q", extra, got)
	}
}

func TestStoreSetCurrentDir_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetCurrentDir("does-not-exist", ""); err == nil {
		t.Fatal("expected error for unknown session")
	}
}
