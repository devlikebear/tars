//go:build !windows

package computeruse

import (
	"os/exec"
	"syscall"
)

// configureCuaDriverProcess mirrors pkg/llm's CLI handling: own process group,
// SIGKILL the group on cancel so a wedged helper cannot hold the pipe open.
func configureCuaDriverProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = cuaDriverWaitDelay
}
