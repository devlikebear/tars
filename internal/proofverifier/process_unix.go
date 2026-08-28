//go:build !windows

package proofverifier

import (
	"os/exec"
	"syscall"
)

// configureCommand runs the verification shell in its own process group and, on
// timeout, SIGKILLs the whole group. A verified command is an arbitrary shell
// string and may background descendants that inherit the stdout pipe; killing
// only the shell leaves them holding it open, so Wait would block past the
// deadline. WaitDelay bounds any residual pipe wait for the descendants that
// survive the group kill anyway.
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the whole process group created by Setpgid.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = commandWaitDelay
}
