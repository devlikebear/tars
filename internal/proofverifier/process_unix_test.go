//go:build !windows

package proofverifier

import (
	"os/exec"
	"testing"
)

// TestConfigureCommandCancelToleratesUnstartedProcess covers the guard in the
// cancel hook. exec.Cmd calls Cancel when the context ends, and that can happen
// before Start ever populated cmd.Process — an already-cancelled context is
// enough. Without the nil check the hook would dereference a nil Process and
// panic inside os/exec rather than letting Run report the timeout.
func TestConfigureCommandCancelToleratesUnstartedProcess(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	configureCommand(cmd)

	if cmd.Cancel == nil {
		t.Fatal("configureCommand left Cancel unset")
	}
	if cmd.WaitDelay != commandWaitDelay {
		t.Fatalf("WaitDelay = %v, want %v", cmd.WaitDelay, commandWaitDelay)
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v, want nil", err)
	}
}
