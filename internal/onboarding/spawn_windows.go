//go:build windows

package onboarding

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureDetachedProcess is the Windows counterpart to setsid. Windows has no
// sessions in the POSIX sense, so detaching means two creation flags:
// DETACHED_PROCESS keeps the child off the parent's console (otherwise closing
// the console window would take the server with it, and a console would flash
// when `tars init` runs from Explorer), and CREATE_NEW_PROCESS_GROUP stops
// Ctrl-C/Ctrl-Break in the parent's console from being delivered to the child.
func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
