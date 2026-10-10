//go:build !windows

package focusprobe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Item 1: an ssh that hangs and leaves a child holding git's output pipe
// must not keep the fetch past its timeout (measured 1m15s before: killing
// git alone left Output waiting on the child).
func TestGitFetchTagsBoundedWhenSSHHangs(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	fakeSSH := filepath.Join(t.TempDir(), "hang-ssh")
	// A child inherits stdout/stderr and outlives the shell's kill.
	script := "#!/bin/sh\nsleep 30 &\nwait\n"
	if err := os.WriteFile(fakeSSH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", fakeSSH)
	// Skip git's "ssh -G" variant probe (its output goes to /dev/null) so
	// the connect itself, whose stderr is git's, is what hangs.
	t.Setenv("GIT_SSH_VARIANT", "ssh")
	gitRun(t, repo, "remote", "add", "origin", "ssh://example.invalid/repo.git")

	prev := releaseFetchTimeout
	releaseFetchTimeout = 300 * time.Millisecond
	t.Cleanup(func() { releaseFetchTimeout = prev })

	start := time.Now()
	ok := FetchTags(context.Background(), repo)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("a hung fetch is not a success")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("fetch took %v, want about timeout + kill grace", elapsed)
	}
}

func TestBatchSSHCommandKeepsThePersonsCommand(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	t.Setenv("GIT_SSH_COMMAND", "")
	if got := batchSSHCommand(context.Background(), repo); got != "ssh "+releaseSSHOptions {
		t.Fatalf("default = %q", got)
	}
	gitRun(t, repo, "config", "core.sshCommand", "ssh -i keyfile")
	if got := batchSSHCommand(context.Background(), repo); got != "ssh -i keyfile "+releaseSSHOptions {
		t.Fatalf("core.sshCommand = %q", got)
	}
	t.Setenv("GIT_SSH_COMMAND", "my-ssh")
	if got := batchSSHCommand(context.Background(), repo); got != "my-ssh "+releaseSSHOptions {
		t.Fatalf("env = %q", got)
	}
}
