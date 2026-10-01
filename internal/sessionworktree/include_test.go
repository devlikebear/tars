package sessionworktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// depsRepo is newRepo plus a gitignored dependency folder shaped like
// node_modules: a package, a relative link into it the way .bin entries are,
// and links that would reach outside the repository.
func depsRepo(t *testing.T) (repo string, outside string) {
	t.Helper()
	repo = newRepo(t)
	outside = filepath.Join(t.TempDir(), "outside")
	write(t, filepath.Join(outside, "secret.txt"), "outside\n")
	write(t, filepath.Join(repo, ".gitignore"), ".env\nlocal/\nnode_modules/\nlinked\n")
	run(t, repo, "commit", "-q", "-am", "ignore deps")
	write(t, filepath.Join(repo, "node_modules", "pkg", "bin.js"), "original\n")
	if err := os.MkdirAll(filepath.Join(repo, "node_modules", ".bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "pkg", "bin.js"), filepath.Join(repo, "node_modules", ".bin", "tool")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "node_modules", "abs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", ".."), filepath.Join(repo, "node_modules", "escape")); err != nil {
		t.Fatal(err)
	}
	return repo, outside
}

func waitDone(t *testing.T, ch <-chan IncludeDone) IncludeDone {
	t.Helper()
	select {
	case done := <-ch:
		return done
	case <-time.After(10 * time.Second):
		t.Fatal("the background copy never finished")
		return IncludeDone{}
	}
}

func TestIncludeFolderIsAnIndependentCopy(t *testing.T) {
	ctx := context.Background()
	repo, _ := depsRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "deps", SourceDir: repo, Include: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	wt := created.Worktree.Path
	if strings.Join(created.Copied, ",") != "node_modules" || len(created.Pending) != 0 {
		t.Fatalf("copied = %v pending = %v", created.Copied, created.Pending)
	}
	// A link that stays inside the repository survives as a link.
	target, err := os.Readlink(filepath.Join(wt, "node_modules", ".bin", "tool"))
	if err != nil || filepath.ToSlash(target) != "../pkg/bin.js" {
		t.Fatalf("relative link = %q, %v", target, err)
	}
	if read(t, filepath.Join(wt, "node_modules", ".bin", "tool")) != "original\n" {
		t.Fatal("the relative link should resolve inside the worktree")
	}
	// Links that leave the repository are dropped and reported.
	for _, name := range []string{"abs", "escape"} {
		if _, err := os.Lstat(filepath.Join(wt, "node_modules", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("link %s pointing outside the repository was copied", name)
		}
	}
	if len(created.Skipped) != 1 || !strings.Contains(created.Skipped[0], "2 symlinks") {
		t.Fatalf("skipped = %v", created.Skipped)
	}
	// What the session does to its copy never reaches the checkout: this
	// is what a symlinked node_modules got wrong.
	write(t, filepath.Join(wt, "node_modules", "pkg", "bin.js"), "changed\n")
	if err := os.RemoveAll(filepath.Join(wt, "node_modules", ".bin")); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(repo, "node_modules", "pkg", "bin.js")) != "original\n" {
		t.Fatal("editing the copy changed the checkout")
	}
	if _, err := os.Lstat(filepath.Join(repo, "node_modules", ".bin", "tool")); err != nil {
		t.Fatal("removing from the copy removed from the checkout")
	}
}

func TestIncludeRefusesPathsThroughSymlinks(t *testing.T) {
	ctx := context.Background()
	repo, outside := depsRepo(t)
	if err := os.Symlink(outside, filepath.Join(repo, "linked")); err != nil {
		t.Fatal(err)
	}
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "links", SourceDir: repo, Include: []string{"linked/secret.txt", "linked", "linked/*", ".git/config", "a/../../x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Copied) != 0 || len(created.Skipped) != 5 {
		t.Fatalf("copied = %v skipped = %v", created.Copied, created.Skipped)
	}
	if _, err := os.Lstat(filepath.Join(created.Worktree.Path, "linked")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("something was copied through a symlink")
	}
}

func TestIncludeRunsInTheBackgroundPastTheWait(t *testing.T) {
	ctx := context.Background()
	repo, _ := depsRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	m.includeWait = 0
	release := make(chan struct{})
	m.copyHook = func(ctx context.Context) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	finished := make(chan IncludeDone, 1)
	m.OnIncludeDone(func(done IncludeDone) { finished <- done })

	created, err := m.Create(ctx, CreateRequest{SessionID: "slow", SourceDir: repo, Include: []string{".env", "node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created.Pending, ",") != ".env,node_modules" || len(created.Copied) != 0 {
		t.Fatalf("pending = %v copied = %v", created.Pending, created.Copied)
	}
	// Nothing half-copied shows up in the worktree while it runs.
	if _, err := os.Lstat(filepath.Join(created.Worktree.Path, "node_modules")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a partial copy is visible in the worktree")
	}
	close(release)
	done := waitDone(t, finished)
	if done.SessionID != "slow" || strings.Join(done.Copied, ",") != ".env,node_modules" {
		t.Fatalf("done = %+v", done)
	}
	if read(t, filepath.Join(created.Worktree.Path, "node_modules", "pkg", "bin.js")) != "original\n" {
		t.Fatal("folder not in place after the background copy")
	}
	if _, err := os.Stat(m.stagingDir(created.Worktree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("staging folder left behind")
	}
}

func TestIncludeNeverReplacesWhatTheSessionMade(t *testing.T) {
	ctx := context.Background()
	repo, _ := depsRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	m.includeWait = 0
	release := make(chan struct{})
	m.copyHook = func(ctx context.Context) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	finished := make(chan IncludeDone, 1)
	m.OnIncludeDone(func(done IncludeDone) { finished <- done })
	created, err := m.Create(ctx, CreateRequest{SessionID: "raced", SourceDir: repo, Include: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	// The session ran its own install before the copy landed.
	write(t, filepath.Join(created.Worktree.Path, "node_modules", "mine.txt"), "mine\n")
	close(release)
	done := waitDone(t, finished)
	if len(done.Copied) != 0 || len(done.Skipped) != 1 || !strings.Contains(done.Skipped[0], "already exists") {
		t.Fatalf("done = %+v", done)
	}
	if read(t, filepath.Join(created.Worktree.Path, "node_modules", "mine.txt")) != "mine\n" {
		t.Fatal("the session's own folder was replaced")
	}
}

func TestDiscardStopsAPendingInclude(t *testing.T) {
	ctx := context.Background()
	repo, _ := depsRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	m.includeWait = 0
	m.copyHook = func(ctx context.Context) { <-ctx.Done() }
	finished := make(chan IncludeDone, 1)
	m.OnIncludeDone(func(done IncludeDone) { finished <- done })
	created, err := m.Create(ctx, CreateRequest{SessionID: "stop", SourceDir: repo, Include: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Discard(ctx, created.Worktree); err != nil {
		t.Fatal(err)
	}
	done := waitDone(t, finished)
	if len(done.Copied) != 0 || len(done.Skipped) != 1 || !strings.Contains(done.Skipped[0], "canceled") {
		t.Fatalf("done = %+v", done)
	}
	for _, path := range []string{created.Worktree.Path, m.stagingDir(created.Worktree)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s left behind", path)
		}
	}
}

func TestIncludeCopyLimitAppliesOnlyWithoutClones(t *testing.T) {
	ctx := context.Background()
	repo, _ := depsRepo(t)
	write(t, filepath.Join(repo, "node_modules", "big.bin"), strings.Repeat("x", 4096))

	noClone := New(filepath.Join(t.TempDir(), "worktrees"))
	noClone.clone = func(src, dst string) error { return errCloneUnsupported }
	noClone.copyLimit = 1024
	created, err := noClone.Create(ctx, CreateRequest{SessionID: "limit", SourceDir: repo, Include: []string{"node_modules", ".env"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created.Copied, ",") != ".env" {
		t.Fatalf("copied = %v", created.Copied)
	}
	if len(created.Skipped) != 1 || !strings.Contains(created.Skipped[0], "copy limit") {
		t.Fatalf("skipped = %v", created.Skipped)
	}
	if _, err := os.Lstat(filepath.Join(created.Worktree.Path, "node_modules")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a folder over the limit was partly copied")
	}

	cloned := 0
	withClone := New(filepath.Join(t.TempDir(), "worktrees"))
	withClone.clone = func(src, dst string) error {
		cloned++
		return plainCopy(src, dst)
	}
	withClone.copyLimit = 1024
	created, err = withClone.Create(ctx, CreateRequest{SessionID: "clone", SourceDir: repo, Include: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created.Copied, ",") != "node_modules" || cloned < 2 {
		t.Fatalf("copied = %v cloned = %d skipped = %v", created.Copied, cloned, created.Skipped)
	}
}

// TestPlatformClone exercises the copy-on-write clone where the file system
// has one (APFS, Btrfs, XFS); elsewhere it must report that it cannot.
func TestPlatformClone(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	write(t, src, "clone me\n")
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.txt")
	if err := cloneFile(src, dst); err != nil {
		if _, statErr := os.Lstat(dst); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("a failed clone left a file behind")
		}
		t.Skip("no copy-on-write clone here:", err)
	}
	if read(t, dst) != "clone me\n" {
		t.Fatal("clone content")
	}
	write(t, dst, "changed\n")
	if read(t, src) != "clone me\n" {
		t.Fatal("writing the clone changed the source")
	}
}
