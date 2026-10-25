package sessionworktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRepoWithRemote makes a clone of a bare "origin" repository with one
// commit pushed to origin/main, the way a real checkout with a remote
// looks. It returns the clone (the "checkout") and the bare upstream path.
func newRepoWithRemote(t *testing.T) (clone, upstream string) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	upstream = filepath.Join(t.TempDir(), "upstream.git")
	if err := os.MkdirAll(upstream, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, upstream, "init", "-q", "--bare", "-b", "main")
	clone = filepath.Join(t.TempDir(), "clone")
	run(t, filepath.Dir(clone), "clone", "-q", upstream, clone)
	run(t, clone, "config", "commit.gpgSign", "false")
	write(t, filepath.Join(clone, "README.md"), "hello\n")
	run(t, clone, "add", "-A")
	run(t, clone, "commit", "-q", "-m", "init")
	run(t, clone, "push", "-q", "origin", "main")
	return clone, upstream
}

// TestCreateBaseRemoteDefault proves the worktree branches from origin's
// default branch's latest commit — fetched fresh, not the checkout's HEAD —
// even when the checkout itself sits on a feature branch with local
// commits the remote never saw (tars dogfooding finding #1).
func TestCreateBaseRemoteDefault(t *testing.T) {
	ctx := context.Background()
	clone, upstream := newRepoWithRemote(t)

	// The checkout moves to a feature branch with a local commit the
	// remote has never seen.
	run(t, clone, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(clone, "wip.txt"), "work in progress\n")
	run(t, clone, "add", "-A")
	run(t, clone, "commit", "-q", "-m", "local wip")

	// Someone else pushes a commit to origin/main that this checkout's
	// local origin/main ref does not know about yet.
	other := filepath.Join(t.TempDir(), "other")
	run(t, filepath.Dir(other), "clone", "-q", upstream, other)
	write(t, filepath.Join(other, "shipped.txt"), "shipped on main\n")
	run(t, other, "add", "-A")
	run(t, other, "commit", "-q", "-m", "shipped on main")
	run(t, other, "push", "-q", "origin", "main")
	shipped := strings.TrimSpace(run(t, other, "rev-parse", "HEAD"))

	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s1", SourceDir: clone, Base: BaseRemoteDefault})
	if err != nil {
		t.Fatal(err)
	}
	if created.Base.Fallback != "" {
		t.Fatalf("fallback = %q, want none", created.Base.Fallback)
	}
	if created.Base.Ref != "origin/main" {
		t.Fatalf("ref = %q, want origin/main", created.Base.Ref)
	}
	if created.Base.Commit != shipped {
		t.Fatalf("base commit = %s, want the fetched origin/main commit %s", created.Base.Commit, shipped)
	}
	wt := created.Worktree
	if wt.BaseCommit != shipped {
		t.Fatalf("worktree base commit = %s, want %s", wt.BaseCommit, shipped)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, "shipped.txt")); err != nil {
		t.Fatalf("worktree should contain origin/main's shipped.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, "wip.txt")); !os.IsNotExist(err) {
		t.Fatalf("worktree should not contain the checkout's own-branch wip.txt (err=%v)", err)
	}
}

// TestCreateBaseRemoteDefaultNoRemote proves a repository with no "origin"
// remote falls back to HEAD and reports why.
func TestCreateBaseRemoteDefaultNoRemote(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	head := strings.TrimSpace(run(t, repo, "rev-parse", "HEAD"))

	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s1", SourceDir: repo, Base: BaseRemoteDefault})
	if err != nil {
		t.Fatal(err)
	}
	if created.Base.Fallback != BaseFallbackNoRemote {
		t.Fatalf("fallback = %q, want %q", created.Base.Fallback, BaseFallbackNoRemote)
	}
	if created.Base.Ref != "" || created.Base.Commit != head {
		t.Fatalf("base = %+v, want HEAD %s with no ref", created.Base, head)
	}
	if created.Worktree.BaseCommit != head {
		t.Fatalf("worktree base commit = %s, want HEAD %s", created.Worktree.BaseCommit, head)
	}
}

// TestCreateBaseRemoteDefaultUnreachableRemote proves an origin remote
// that cannot be fetched falls back to HEAD and reports why, instead of
// failing the whole creation.
func TestCreateBaseRemoteDefaultUnreachableRemote(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	head := strings.TrimSpace(run(t, repo, "rev-parse", "HEAD"))
	run(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s1", SourceDir: repo, Base: BaseRemoteDefault})
	if err != nil {
		t.Fatal(err)
	}
	if created.Base.Fallback != BaseFallbackFetchFailed {
		t.Fatalf("fallback = %q, want %q", created.Base.Fallback, BaseFallbackFetchFailed)
	}
	if created.Base.Commit != head {
		t.Fatalf("base commit = %s, want HEAD %s", created.Base.Commit, head)
	}
}

// TestCreateBaseDefaultIsHEAD proves the default (no Base set) keeps
// branching from HEAD, exactly as before this change — a plain chat
// session's isolation never asks for BaseRemoteDefault.
func TestCreateBaseDefaultIsHEAD(t *testing.T) {
	ctx := context.Background()
	clone, _ := newRepoWithRemote(t)
	run(t, clone, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(clone, "wip.txt"), "work in progress\n")
	run(t, clone, "add", "-A")
	run(t, clone, "commit", "-q", "-m", "local wip")
	head := strings.TrimSpace(run(t, clone, "rev-parse", "HEAD"))

	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s1", SourceDir: clone})
	if err != nil {
		t.Fatal(err)
	}
	if created.Base.Ref != "" || created.Base.Fallback != "" {
		t.Fatalf("base = %+v, want plain HEAD with no ref/fallback", created.Base)
	}
	if created.Worktree.BaseCommit != head {
		t.Fatalf("worktree base commit = %s, want checkout HEAD %s", created.Worktree.BaseCommit, head)
	}
	if _, err := os.Stat(filepath.Join(created.Worktree.Path, "wip.txt")); err != nil {
		t.Fatalf("worktree should keep branching from HEAD: %v", err)
	}
}
