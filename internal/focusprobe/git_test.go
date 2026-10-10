package focusprobe

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// commitRepoFile writes and commits one file, returning the new HEAD.
func commitRepoFile(t *testing.T, repo, name, subject string) string {
	t.Helper()
	writeRepoFile(t, repo, name, name+"\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", subject)
	return strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
}

func TestIsGitObjectID(t *testing.T) {
	for id, want := range map[string]bool{
		"91cf959d": true,
		"91cf959d0c0d6a3c4a2f3b1e5d7c9a8b6f4e2d10": true,
		"ABCDEF1":             true,
		"":                    false,
		"abc123":              false, // shorter than git's shortest abbreviation
		"HEAD":                false,
		"--help":              false,
		"-n":                  false,
		"91cf959d --output=x": false,
		"91cf959g":            false,
	} {
		if got := IsGitObjectID(id); got != want {
			t.Errorf("IsGitObjectID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestTagHasCommit(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	tagged := commitRepoFile(t, repo, "b.txt", "b")
	gitRun(t, repo, "tag", "v1.0.0")
	later := commitRepoFile(t, repo, "c.txt", "c")

	if !TagHasCommit(ctx, repo, "v1.0.0", tagged) {
		t.Error("the tagged commit is not in its own tag")
	}
	if !TagHasCommit(ctx, repo, "v1.0.0", tagged[:8]) {
		t.Error("an abbreviated commit id is refused")
	}
	if TagHasCommit(ctx, repo, "v1.0.0", later) {
		t.Error("a commit after the tag is reported as in it")
	}
	for _, commit := range []string{"", "--help", "-h", "v1.0.0"} {
		if TagHasCommit(ctx, repo, "v1.0.0", commit) {
			t.Errorf("commit %q is reported as in the tag", commit)
		}
	}
}

func TestLatestReleaseTagAndItsMergedPRs(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	if _, ok := LatestReleaseTag(ctx, repo); ok {
		t.Fatal("a repository without tags reported a release tag")
	}

	commitRepoFile(t, repo, "squash.txt", "feat: squash merged (#12)")
	commitRepoFile(t, repo, "merge.txt", "Merge pull request #13 from someone/branch")
	commitRepoFile(t, repo, "mention.txt", "fix: follow up to #14 in the body only")
	gitRun(t, repo, "tag", "v1.0.0")
	commitRepoFile(t, repo, "after.txt", "feat: after the tag (#15)")
	gitRun(t, repo, "tag", "not-a-release")

	tag, ok := LatestReleaseTag(ctx, repo)
	if !ok || tag.Name != "v1.0.0" || tag.At.IsZero() {
		t.Fatalf("latest release tag = %+v, %v; want v1.0.0", tag, ok)
	}
	prs := TagMergedPRs(ctx, repo, tag.Name)
	if !prs[12] || !prs[13] {
		t.Errorf("merged pull requests in the tag = %v, want 12 and 13", prs)
	}
	if prs[14] || prs[15] {
		t.Errorf("merged pull requests in the tag = %v, want neither 14 (a mention) nor 15 (after the tag)", prs)
	}
	if got := TagMergedPRs(ctx, repo, "no-such-tag"); got == nil || len(got) != 0 {
		t.Errorf("merged pull requests of a missing tag = %v, want an empty map", got)
	}
}

func TestMainCheckout(t *testing.T) {
	ctx := context.Background()
	repo, _ := focusReviewRepo(t)
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	same := func(got string) bool {
		resolved, err := filepath.EvalSymlinks(got)
		return err == nil && resolved == want
	}
	if got := MainCheckout(ctx, repo); !same(got) {
		t.Errorf("main checkout of the repository = %q, want %q", got, want)
	}

	// A linked worktree shares the repository's git folder, so its main
	// checkout is the repository, not the worktree.
	linked := filepath.Join(t.TempDir(), "linked")
	gitRun(t, repo, "worktree", "add", "-q", "-b", "side", linked)
	if got := MainCheckout(ctx, linked); !same(got) {
		t.Errorf("main checkout of a linked worktree = %q, want %q", got, want)
	}

	if got := MainCheckout(ctx, t.TempDir()); got != "" {
		t.Errorf("main checkout outside a repository = %q, want none", got)
	}
}

func TestTagFetchCacheFetchesOncePerWindow(t *testing.T) {
	calls := map[string]int{}
	cache := NewTagFetchCache(func(_ context.Context, repo string) bool {
		calls[repo]++
		return repo != "broken"
	})
	clock := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cache.Now = func() time.Time { return clock }
	ctx := context.Background()

	first := cache.Fetched(ctx, "a")
	second := cache.Fetched(ctx, "a")
	if !first || !second {
		t.Fatalf("a successful fetch was reported as failed: first=%v second=%v", first, second)
	}
	if calls["a"] != 1 {
		t.Fatalf("fetches of one repository within the window = %d, want 1", calls["a"])
	}
	if cache.Fetched(ctx, "broken") {
		t.Fatal("a failed fetch was reported as successful")
	}
	if !cache.Fetched(ctx, "b") || calls["b"] != 1 {
		t.Fatalf("another repository shares the first one's result: calls = %v", calls)
	}

	clock = clock.Add(FetchWindow + time.Second)
	if !cache.Fetched(ctx, "a") || calls["a"] != 2 {
		t.Fatalf("fetches after the window = %d, want 2", calls["a"])
	}
}
