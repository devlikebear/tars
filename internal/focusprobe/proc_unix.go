//go:build !windows

package focusprobe

import (
	"os/exec"
	"syscall"
	"time"
)

// KillProcessGroupOnCancel runs cmd in a process group of its own and, on
// cancel, sends the group SIGTERM and then SIGKILL after grace, so children
// (git's ssh) die with it and stop holding its output pipes.
func KillProcessGroupOnCancel(cmd *exec.Cmd, grace time.Duration) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		_ = syscall.Kill(pgid, syscall.SIGTERM)
		time.AfterFunc(grace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		return nil
	}
}
