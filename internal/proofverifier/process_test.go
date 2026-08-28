package proofverifier

import (
	"context"
	"testing"
	"time"
)

// commandTestBudget bounds the shell invocations in this package's tests that
// are not themselves testing timing. It is deliberately generous: commands go
// through a *login* shell (`-lc`), and on a cold Windows runner Git Bash spends
// most of a second sourcing profile files before the command even starts. A
// one-second budget made those call sites fail under CI load, and the timeout
// path they fell into then exposed the descendant-pipe hang that
// TestProcessRunnerReturnsWhenDescendantOutlivesTimeout pins.
const commandTestBudget = 30 * time.Second

// TestProcessRunnerReturnsWhenDescendantOutlivesTimeout pins the deadline
// contract that matters in production: Run must come back when the timeout
// fires, even if the command left a descendant behind.
//
// The command string reaches a POSIX shell, so a background descendant inherits
// the shell's stdout pipe. Killing only the shell leaves that pipe's write end
// open, and os/exec's Wait blocks on its output-copying goroutine until every
// writer closes — so without a bounded wait, Run hangs for as long as the
// descendant lives rather than returning at the deadline. That is what stalled
// the Windows CI job for the full 5-minute test timeout.
func TestProcessRunnerReturnsWhenDescendantOutlivesTimeout(t *testing.T) {
	t.Parallel()

	// The descendant must outlive both the command timeout and the wait delay
	// by enough that a hang is unambiguous rather than a slow success.
	const commandTimeout = 200 * time.Millisecond
	const descendantLifetime = 30 // seconds

	done := make(chan CommandResult, 1)
	failed := make(chan error, 1)
	go func() {
		result, err := (processCommandRunner{}).Run(
			context.Background(),
			t.TempDir(),
			"sleep 30 & sleep 30",
			commandTimeout,
		)
		if err != nil {
			failed <- err
			return
		}
		done <- result
	}()

	// Generous relative to commandTimeout + commandWaitDelay, but far below the
	// descendant's lifetime: landing here means the pipe wait is bounded.
	budget := commandTimeout + commandWaitDelay + 5*time.Second
	select {
	case err := <-failed:
		t.Fatalf("run command: %v", err)
	case result := <-done:
		if !result.TimedOut || result.ExitCode != -1 {
			t.Fatalf("expected a timed-out result, got %+v", result)
		}
	case <-time.After(budget):
		t.Fatalf("Run did not return within %v; it is waiting on a descendant that lives %ds", budget, descendantLifetime)
	}
}
