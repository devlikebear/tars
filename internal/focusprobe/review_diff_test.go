package focusprobe

import (
	"context"
	"strings"
	"testing"
)

func TestReviewDiffOf(t *testing.T) {
	repo, base := focusReviewRepo(t)
	// A committed change, an uncommitted one, and a file git does not track.
	writeRepoFile(t, repo, "c.go", "package c\n")
	gitRun(t, repo, "add", "c.go")
	gitRun(t, repo, "commit", "-q", "-m", "add c")
	writeRepoFile(t, repo, "b.go", "line 1\nchanged\n")
	writeRepoFile(t, repo, "new/d.go", "package d\n")
	writeRepoFile(t, repo, ".gitignore", "*.log\n")
	writeRepoFile(t, repo, "skip.log", "noise\n")

	got, ok := ReviewDiffOf(context.Background(), repo, base)
	if !ok || got.Base != base || got.Truncated || got.Stat != "" {
		t.Fatalf("diff = %+v ok = %v", got, ok)
	}
	for _, want := range []string{"+package c", "+changed", "-line 2"} {
		if !strings.Contains(got.Diff, want) {
			t.Fatalf("diff misses %q:\n%s", want, got.Diff)
		}
	}
	if strings.Join(got.Untracked, ",") != ".gitignore,new/d.go" {
		t.Fatalf("untracked = %v", got.Untracked)
	}
	if got.Empty() {
		t.Fatal("not empty")
	}
}

func TestReviewDiffOfTruncates(t *testing.T) {
	repo, base := focusReviewRepo(t)
	big := strings.Repeat("a line that makes the diff long\n", ReviewDiffMaxBytes/16)
	writeRepoFile(t, repo, "big.txt", big)
	gitRun(t, repo, "add", "big.txt")
	got, ok := ReviewDiffOf(context.Background(), repo, base)
	if !ok || !got.Truncated || len(got.Diff) > ReviewDiffMaxBytes || !strings.HasSuffix(got.Diff, "long") {
		t.Fatalf("truncated = %v len = %d", got.Truncated, len(got.Diff))
	}
	if !strings.Contains(got.Stat, "big.txt") {
		t.Fatalf("stat = %q", got.Stat)
	}
}

func TestReviewDiffOfBaseAndFolder(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	writeRepoFile(t, repo, "sub/e.go", "package e\n")
	gitRun(t, repo, "add", "sub/e.go")
	// An option-shaped base is never passed to git: the diff starts at HEAD.
	got, ok := ReviewDiffOf(context.Background(), repo+"/sub", "--output=out.txt")
	if !ok || got.Base != "HEAD" || !strings.Contains(got.Diff, "+package e") {
		t.Fatalf("diff = %+v ok = %v", got, ok)
	}
	if _, ok := ReviewDiffOf(context.Background(), t.TempDir(), ""); ok {
		t.Fatal("a folder outside git has no review diff")
	}
	if _, ok := ReviewDiffOf(context.Background(), "", ""); ok {
		t.Fatal("no folder has no review diff")
	}
	if !(ReviewDiff{}).Empty() {
		t.Fatal("zero diff is empty")
	}
}
