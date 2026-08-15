package skillhub

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/shellexec"
)

// TestSmokeCommandFailsWhenNoShellIsAvailable covers the branch that keeps a
// missing shell from being mistaken for a failing skill. shellexec.Executable
// is a func var so this seam exists; the test must not run in parallel with
// anything that shells out.
func TestSmokeCommandFailsWhenNoShellIsAvailable(t *testing.T) {
	previous := shellexec.Executable
	shellexec.Executable = func() (string, error) {
		return "", fmt.Errorf("%w: nothing installed", shellexec.ErrShellUnavailable)
	}
	t.Cleanup(func() { shellexec.Executable = previous })

	check := runSkillSmokeCommand(context.Background(), t.TempDir(), t.TempDir(), 0, "echo hello")
	if check.Status != SandboxCheckFailed {
		t.Fatalf("status = %q, want %q", check.Status, SandboxCheckFailed)
	}
	if !strings.Contains(check.Error, shellexec.ErrShellUnavailable.Error()) {
		t.Fatalf("expected the error to name the missing shell, got %q", check.Error)
	}
}
