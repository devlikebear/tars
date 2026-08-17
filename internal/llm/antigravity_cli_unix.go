//go:build !windows

package llm

import (
	"os/exec"
	"syscall"
)

// configureAntigravityCLIProcess runs agy in its own process group and, on
// context cancellation, SIGKILLs the whole group. The CLI spawns descendants
// (e.g. stdio MCP servers) that inherit the stdout pipe; killing only the
// direct child leaves them holding the pipe open, so the stream read would
// block past the deadline. WaitDelay bounds any residual pipe wait.
func configureAntigravityCLIProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the whole process group created by Setpgid.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = antigravityCLIWaitDelay
}
