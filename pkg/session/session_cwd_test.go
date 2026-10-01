package session

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestStoreSwitchCurrentDir_AddsExistingDirectory(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	repo := testCanonicalPath(t, filepath.Join(root, "some", "repo"))
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	added, err := store.SwitchCurrentDir(sess.ID, repo)
	if err != nil {
		t.Fatalf("switch cwd: %v", err)
	}
	if !added {
		t.Fatal("expected the directory to be registered")
	}
	cur, _ := store.GetCurrentDir(sess.ID)
	if cur != repo {
		t.Fatalf("expected current %q, got %q", repo, cur)
	}
	eligible, _ := store.EligibleCwds(sess.ID)
	artifact := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	if want := []string{artifact, repo}; !reflect.DeepEqual(eligible, want) {
		t.Fatalf("eligible mismatch: got=%v want=%v", eligible, want)
	}

	// Switching back to an existing candidate registers nothing new.
	added, err = store.SwitchCurrentDir(sess.ID, artifact)
	if err != nil || added {
		t.Fatalf("switch back: added=%v err=%v", added, err)
	}
	if eligible, _ = store.EligibleCwds(sess.ID); len(eligible) != 2 {
		t.Fatalf("expected 2 candidates, got %v", eligible)
	}
}

func TestStoreSwitchCurrentDir_RejectsInvalidTargets(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	before, _ := store.GetCurrentDir(sess.ID)

	cases := []struct {
		name string
		dir  string
		want error
	}{
		{"missing", filepath.Join(root, "nope"), ErrCwdNotFound},
		{"file", file, ErrCwdNotDirectory},
		{"relative", "some/repo", ErrCwdNotAbsolute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			added, err := store.SwitchCurrentDir(sess.ID, tc.dir)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if added {
				t.Fatal("rejected target must not be registered")
			}
		})
	}
	after, _ := store.GetCurrentDir(sess.ID)
	eligible, _ := store.EligibleCwds(sess.ID)
	if after != before || len(eligible) != 1 {
		t.Fatalf("rejections changed state: current=%q eligible=%v", after, eligible)
	}
}

func TestStoreSwitchCurrentDir_ReportsUnreadableTarget(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions enforced for the test user")
	}
	root := t.TempDir()
	store := NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	added, err := store.SwitchCurrentDir(sess.ID, filepath.Join(locked, "repo"))
	if err == nil || errors.Is(err, ErrCwdNotFound) || added {
		t.Fatalf("expected an access error without registering, got added=%v err=%v", added, err)
	}
}

func TestStoreSwitchCurrentDir_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.SwitchCurrentDir("does-not-exist", t.TempDir()); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
