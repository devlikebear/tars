//go:build !windows

package onboarding

import (
	"os/exec"
	"syscall"
)

// configureDetachedProcess puts the child in its own session so it is not
// killed when the parent's terminal or process group goes away.
func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
