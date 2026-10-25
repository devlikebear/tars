package focusprobe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkTreeStateUnchanged proves two reads with nothing in between
// agree, the core property a review-stage fix turn's git-state judgment
// relies on (tars dogfooding fix #2).
func TestWorkTreeStateUnchanged(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	a, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	b, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b {
		t.Fatalf("state changed with nothing in between: %q vs %q", a, b)
	}
}

// TestWorkTreeStateTrackedEdit proves editing a tracked file changes the
// state.
func TestWorkTreeStateTrackedEdit(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	before, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, "b.go", "changed\n")
	after, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatalf("state unchanged after editing a tracked file: %q", after)
	}
}

// TestWorkTreeStateNewUntrackedFile proves a new untracked file changes
// the state — a fix turn that only adds a file still counts as a change.
func TestWorkTreeStateNewUntrackedFile(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	before, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, "new.go", "package b\n")
	after, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatalf("state unchanged after a new untracked file: %q", after)
	}
}

// TestWorkTreeStateCommit proves a commit changes the state (HEAD moved),
// even when it is the exact content already on disk (a fix turn that
// commits without editing anything further).
func TestWorkTreeStateCommit(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	writeRepoFile(t, repo, "b.go", "changed\n")
	before, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "commit the pending edit")
	after, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatalf("state unchanged after a commit: %q", after)
	}
}

// TestWorkTreeStateIgnoresGitignoredFiles proves a gitignored file never
// moves the state: a fix turn's own scratch files (build output, logs)
// must not look like a real change.
func TestWorkTreeStateIgnoresGitignoredFiles(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	writeRepoFile(t, repo, ".gitignore", "ignored.txt\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "add gitignore")
	before, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, "ignored.txt", "scratch\n")
	after, err := WorkTreeState(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("state changed from a gitignored file: before %q after %q", before, after)
	}
}

// TestWorkTreeStateLeavesIndexAndRepositoryAlone proves the probe never
// touches the real index (staged changes stay exactly as they were) or
// moves any ref.
func TestWorkTreeStateLeavesIndexAndRepositoryAlone(t *testing.T) {
	ctx := context.Background()
	repo, head := focusReviewRepo(t)
	writeRepoFile(t, repo, "staged.txt", "staged\n")
	gitRun(t, repo, "add", "staged.txt")
	stagedBefore := gitRun(t, repo, "diff", "--cached", "--name-only")
	headBefore := strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))

	if _, err := WorkTreeState(ctx, repo); err != nil {
		t.Fatal(err)
	}

	if staged := gitRun(t, repo, "diff", "--cached", "--name-only"); staged != stagedBefore {
		t.Fatalf("real index changed: before %q after %q", stagedBefore, staged)
	}
	if headAfter := strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD")); headAfter != headBefore || headAfter != head {
		t.Fatalf("HEAD moved: before %q after %q", headBefore, headAfter)
	}
	if branches := gitRun(t, repo, "branch", "--list"); strings.Count(branches, "\n") != 1 {
		t.Fatalf("a branch appeared: %q", branches)
	}
}

// TestWorkTreeStateNotARepository proves a non-repository folder is an
// error, not an empty "unchanged" value — a caller must treat the error as
// unknown, never as proof nothing changed.
func TestWorkTreeStateNotARepository(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WorkTreeState(ctx, dir); err == nil {
		t.Fatal("want an error for a non-repository folder")
	}
}
