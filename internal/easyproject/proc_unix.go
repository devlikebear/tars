//go:build !windows

package easyproject

import (
	"os/exec"
	"syscall"
	"time"
)

// stopGroupOnCancel runs cmd in a process group of its own and, on cancel,
// sends the group SIGTERM and then SIGKILL after grace: `npm run dev`
// starts Vite as a child, which would outlive npm and keep the port.
func stopGroupOnCancel(cmd *exec.Cmd, grace time.Duration) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		_ = syscall.Kill(pgid, syscall.SIGTERM)
		time.AfterFunc(grace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		return nil
	}
}
