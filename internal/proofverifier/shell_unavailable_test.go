package proofverifier

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/devlikebear/tars/internal/shellexec"
)

// TestProcessRunnerFailsWhenNoShellIsAvailable covers the branch that reports a
// missing shell as an error rather than a failed verification, which would
// otherwise look like the verified command itself did not pass.
func TestProcessRunnerFailsWhenNoShellIsAvailable(t *testing.T) {
	previous := shellexec.Executable
	shellexec.Executable = func() (string, error) {
		return "", fmt.Errorf("%w: nothing installed", shellexec.ErrShellUnavailable)
	}
	t.Cleanup(func() { shellexec.Executable = previous })

	_, err := (processCommandRunner{}).Run(context.Background(), t.TempDir(), "printf ok", commandTestBudget)
	if !errors.Is(err, shellexec.ErrShellUnavailable) {
		t.Fatalf("expected ErrShellUnavailable, got %v", err)
	}
}
