package sessionworktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// newRepo makes a repository with one commit, a subfolder, and a gitignored
// .env, the way a project checkout looks.
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	dir := filepath.Join(t.TempDir(), "my project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", "main")
	run(t, dir, "config", "commit.gpgSign", "false")
	// Git for Windows defaults to autocrlf=true; tests that compare bytes
	// pin it, and TestApplyKeepsCheckoutLineEndings covers it on.
	run(t, dir, "config", "core.autocrlf", "false")
	write(t, filepath.Join(dir, "README.md"), "hello\n")
	write(t, filepath.Join(dir, "app", "main.txt"), "one\ntwo\nthree\n")
	write(t, filepath.Join(dir, ".gitignore"), ".env\nlocal/\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", "init")
	write(t, filepath.Join(dir, ".env"), "SECRET=1\n")
	write(t, filepath.Join(dir, "local", "cfg.json"), "{}\n")
	return dir
}

func TestCreateApplyRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))

	created, err := m.Create(ctx, CreateRequest{SessionID: "s1", SourceDir: filepath.Join(repo, "app"), Include: []string{".env", "local", "missing.txt", "../outside", "/etc/passwd"}, Reason: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	wt := created.Worktree
	if wt.Branch != "tars/session-s1" || wt.Reason != "manual" || wt.BaseCommit == "" {
		t.Fatalf("worktree = %+v", wt)
	}
	if filepath.Base(wt.Dir) != "app" || !strings.HasPrefix(wt.Dir, wt.Path) {
		t.Fatalf("dir %s should mirror app/ inside %s", wt.Dir, wt.Path)
	}
	if strings.Join(created.Copied, ",") != ".env,local" {
		t.Fatalf("copied = %v", created.Copied)
	}
	if len(created.Skipped) != 3 {
		t.Fatalf("skipped = %v", created.Skipped)
	}
	if read(t, filepath.Join(wt.Path, ".env")) != "SECRET=1\n" {
		t.Fatal(".env not copied")
	}

	// The session edits a file, adds one, and commits one change.
	write(t, filepath.Join(wt.Dir, "main.txt"), "one\nTWO\nthree\n")
	run(t, wt.Path, "commit", "-q", "-am", "session edit")
	write(t, filepath.Join(wt.Dir, "new.txt"), "fresh\n")
	write(t, filepath.Join(wt.Path, "README.md"), "hello\nworld\n")

	status, err := m.Status(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if status.Commits != 1 || strings.Join(status.Files, ",") != "README.md,app/main.txt,app/new.txt" {
		t.Fatalf("status = %+v", status)
	}
	records, _ := m.Records()
	if len(records) != 1 || records[0].SessionID != "s1" {
		t.Fatalf("records = %+v", records)
	}

	files, err := m.Apply(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("applied = %v", files)
	}
	if read(t, filepath.Join(repo, "app", "main.txt")) != "one\nTWO\nthree\n" || read(t, filepath.Join(repo, "app", "new.txt")) != "fresh\n" {
		t.Fatal("changes not applied to the checkout")
	}
	// Applied as working-tree edits: nothing staged or committed.
	if staged := run(t, repo, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("checkout index changed: %q", staged)
	}
	if log := run(t, repo, "log", "--oneline"); strings.Count(log, "\n") != 1 {
		t.Fatalf("checkout history changed: %s", log)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("worktree folder left behind")
	}
	if branches := run(t, repo, "branch", "--list", "tars/*"); branches != "" {
		t.Fatalf("branch left behind: %s", branches)
	}
	if records, _ := m.Records(); len(records) != 0 {
		t.Fatalf("record left behind: %+v", records)
	}
}

func TestApplyConflictLeavesEverything(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s2", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	wt := created.Worktree
	write(t, filepath.Join(wt.Path, "app", "main.txt"), "one\nsession\nthree\n")
	write(t, filepath.Join(repo, "app", "main.txt"), "one\ncheckout\nthree\n")

	_, err = m.Apply(ctx, wt)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(repo, "app", "main.txt")) != "one\ncheckout\nthree\n" {
		t.Fatal("checkout changed on conflict")
	}
	if read(t, filepath.Join(wt.Path, "app", "main.txt")) != "one\nsession\nthree\n" {
		t.Fatal("worktree changed on conflict")
	}
}

func TestApplyWithNothingChanged(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s0", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	files, err := m.Apply(ctx, created.Worktree)
	if err != nil || len(files) != 0 {
		t.Fatalf("files=%v err=%v", files, err)
	}
}

func TestKeepCommitsToTheBranch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s3", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	wt := created.Worktree
	write(t, filepath.Join(wt.Path, "notes.md"), "kept\n")

	branch, err := m.Keep(ctx, wt, "session notes")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "tars/session-s3" {
		t.Fatalf("branch = %s", branch)
	}
	if got := run(t, repo, "show", branch+":notes.md"); got != "kept\n" {
		t.Fatalf("branch content = %q", got)
	}
	if msg := run(t, repo, "log", "-1", "--format=%s", branch); strings.TrimSpace(msg) != "session notes" {
		t.Fatalf("commit message = %q", msg)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("worktree folder left behind")
	}
	if _, err := os.Stat(filepath.Join(repo, "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("keep must not touch the checkout")
	}

	// A second session gets the next free branch name if one is taken.
	again, err := m.Create(ctx, CreateRequest{SessionID: "s3", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if again.Worktree.Branch != "tars/session-s3-2" {
		t.Fatalf("branch = %s", again.Worktree.Branch)
	}
	// Keeping with nothing new commits nothing.
	if _, err := m.Keep(ctx, again.Worktree, ""); err != nil {
		t.Fatal(err)
	}
	if count := run(t, repo, "rev-list", "--count", "main.."+again.Worktree.Branch); strings.TrimSpace(count) != "0" {
		t.Fatalf("empty keep committed: %s", count)
	}
}

func TestDiscardDropsEverything(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s4", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(created.Worktree.Path, "scratch.txt"), "x\n")
	if err := m.Discard(ctx, created.Worktree); err != nil {
		t.Fatal(err)
	}
	if branches := run(t, repo, "branch", "--list", "tars/*"); branches != "" {
		t.Fatalf("branch left: %s", branches)
	}
	if list := run(t, repo, "worktree", "list"); strings.Count(list, "\n") != 1 {
		t.Fatalf("worktree still registered: %s", list)
	}
}

func TestRemoveRefusesPathsOutsideTheRoot(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "s5", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	forged := created.Worktree
	forged.Path = repo
	if err := m.Discard(ctx, forged); err == nil {
		t.Fatal("discard of a path outside the root must fail")
	}
	if _, err := os.Stat(filepath.Join(repo, "README.md")); err != nil {
		t.Fatal("checkout was touched")
	}
}

func TestCreateErrors(t *testing.T) {
	ctx := context.Background()
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	if _, err := m.Create(ctx, CreateRequest{SessionID: "../x", SourceDir: t.TempDir()}); err == nil {
		t.Fatal("bad session id")
	}
	if _, err := m.Create(ctx, CreateRequest{SessionID: "s", SourceDir: t.TempDir()}); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("plain folder err = %v", err)
	}
	empty := t.TempDir()
	run(t, empty, "init", "-q")
	if _, err := m.Create(ctx, CreateRequest{SessionID: "s", SourceDir: empty}); !errors.Is(err, ErrNoCommits) {
		t.Fatalf("empty repo err = %v", err)
	}
	repo := newRepo(t)
	if _, err := m.Create(ctx, CreateRequest{SessionID: "dup", SourceDir: repo}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, CreateRequest{SessionID: "dup", SourceDir: repo}); err == nil {
		t.Fatal("second worktree for one session")
	}
	if got, err := RepoRoot(ctx, filepath.Join(repo, "app")); err != nil || filepath.Base(got) != "my project" {
		t.Fatalf("RepoRoot = %q %v", got, err)
	}
	if m.Root() == "" {
		t.Fatal("root")
	}
}

func TestApplyKeepsCheckoutLineEndings(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	run(t, repo, "config", "core.autocrlf", "true")
	// Re-checkout so the checkout has the CRLF files autocrlf gives it.
	if err := os.Remove(filepath.Join(repo, "app", "main.txt")); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "checkout", "--", "app/main.txt")
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "crlf", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	wtFile := filepath.Join(created.Worktree.Path, "app", "main.txt")
	if got := read(t, wtFile); got != "one\r\ntwo\r\nthree\r\n" {
		t.Fatalf("worktree checkout = %q", got)
	}
	write(t, wtFile, "one\r\nTWO\r\nthree\r\n")
	if _, err := m.Apply(ctx, created.Worktree); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo, "app", "main.txt")); got != "one\r\nTWO\r\nthree\r\n" {
		t.Fatalf("checkout after apply = %q", got)
	}
}

func TestKeepUsesTheConfiguredIdentityAndCopiesFolders(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	run(t, repo, "config", "user.name", "Person")
	run(t, repo, "config", "user.email", "person@example.com")
	// An empty variable is an identity of its own; unset them so the
	// repository's config is what git reads.
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	if err := os.Symlink(filepath.Join(repo, ".env"), filepath.Join(repo, "local", "link.env")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(filepath.Join(repo, ".env"), filepath.Join(repo, "top.link")); err != nil {
		t.Fatal(err)
	}
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "id1", SourceDir: repo, Include: []string{"local", "top.link", "  "}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(created.Worktree.Path, "local", "link.env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a symlink inside a copied folder must not be copied")
	}
	// The top-level link is refused and the link inside local/, which
	// points outside the repository, is dropped from the copy.
	if len(created.Skipped) != 2 || !strings.Contains(strings.Join(created.Skipped, "|"), "top.link: symlinks are not followed") {
		t.Fatalf("skipped = %v", created.Skipped)
	}
	write(t, filepath.Join(created.Worktree.Path, "x.txt"), "x\n")
	branch, err := m.Keep(ctx, created.Worktree, "")
	if err != nil {
		t.Fatal(err)
	}
	if who := run(t, repo, "log", "-1", "--format=%an <%ae>", branch); strings.TrimSpace(who) != "Person <person@example.com>" {
		t.Fatalf("author = %q", who)
	}
}

func TestRecordsIgnoresBrokenFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	m := New(root)
	if records, err := m.Records(); err != nil || len(records) != 0 {
		t.Fatalf("empty root: %v %v", records, err)
	}
	write(t, filepath.Join(root, "abc", "broken.json"), "{not json")
	write(t, filepath.Join(root, "abc", "empty.json"), "{}")
	if records, err := m.Records(); err != nil || len(records) != 0 {
		t.Fatalf("broken records: %v %v", records, err)
	}
	if (&ConflictError{Detail: "x"}).Error() == "" {
		t.Fatal("conflict message")
	}
}

func TestStatusAndApplyFailOnAMissingWorktree(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	m := New(filepath.Join(t.TempDir(), "worktrees"))
	created, err := m.Create(ctx, CreateRequest{SessionID: "gone", SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	wt := created.Worktree
	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Status(ctx, wt); err == nil {
		t.Fatal("status of a missing worktree")
	}
	if _, err := m.Apply(ctx, wt); err == nil {
		t.Fatal("apply of a missing worktree")
	}
	if _, err := m.Keep(ctx, wt, ""); err == nil {
		t.Fatal("keep of a missing worktree")
	}
	// Discard still cleans up the registration and branch.
	if err := m.Discard(ctx, wt); err != nil {
		t.Fatal(err)
	}
	if branches := run(t, repo, "branch", "--list", "tars/*"); branches != "" {
		t.Fatalf("branch left: %s", branches)
	}
}
