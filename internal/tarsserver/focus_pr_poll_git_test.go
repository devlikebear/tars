package tarsserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRepo makes a repository with one commit and returns its folder.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	commitFile(t, dir, "a.txt", "a\n")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgSign=false"}, args...)...) // NOSONAR: the machine's own git
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

func commitFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", name)
	head, err := runGit(context.Background(), releaseGitTimeout, dir, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestGitDiscardCheck(t *testing.T) {
	ctx := context.Background()
	t.Run("clean at the merged head", func(t *testing.T) {
		dir := gitRepo(t)
		head := commitFile(t, dir, "b.txt", "b\n")
		if reason := gitDiscardCheck(ctx, dir, head); reason != "" {
			t.Fatalf("reason = %q", reason)
		}
	})
	t.Run("behind the merged head", func(t *testing.T) {
		dir := gitRepo(t)
		head := commitFile(t, dir, "b.txt", "b\n")
		gitIn(t, dir, "reset", "-q", "--hard", "HEAD~1")
		if reason := gitDiscardCheck(ctx, dir, head); reason != "" {
			t.Fatalf("reason = %q", reason)
		}
	})
	t.Run("uncommitted changes", func(t *testing.T) {
		dir := gitRepo(t)
		head := commitFile(t, dir, "b.txt", "b\n")
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
		if reason := gitDiscardCheck(ctx, dir, head); reason != "the worktree has uncommitted changes" {
			t.Fatalf("reason = %q", reason)
		}
	})
	t.Run("a commit the PR does not hold", func(t *testing.T) {
		dir := gitRepo(t)
		head := commitFile(t, dir, "b.txt", "b\n")
		commitFile(t, dir, "later.txt", "later\n")
		if reason := gitDiscardCheck(ctx, dir, head); reason != "the worktree has commits that are not in the merged pull request" {
			t.Fatalf("reason = %q", reason)
		}
	})
	t.Run("a merged head this repository never fetched", func(t *testing.T) {
		// c25: e.g. GitHub's "Update branch" added a commit remotely.
		dir := gitRepo(t)
		elsewhere := commitFile(t, gitRepo(t), "remote.txt", "r\n")
		if reason := gitDiscardCheck(ctx, dir, elsewhere); reason != "the merged head is not in the local repository; fetch it to compare" {
			t.Fatalf("reason = %q", reason)
		}
	})
	t.Run("unknown head", func(t *testing.T) {
		if reason := gitDiscardCheck(ctx, gitRepo(t), ""); reason == "" {
			t.Fatal("discard without a head")
		}
	})
}

func TestGitLocalBranch(t *testing.T) {
	dir := gitRepo(t)
	if got := gitLocalBranch(context.Background(), dir); got != "main" {
		t.Fatalf("branch = %q", got)
	}
	gitIn(t, dir, "checkout", "-q", "--detach")
	if got := gitLocalBranch(context.Background(), dir); got != "" {
		t.Fatalf("detached = %q", got)
	}
}
