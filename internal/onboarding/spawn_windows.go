//go:build windows

package onboarding

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureDetachedProcess is the Windows counterpart to setsid. Windows has no
// sessions in the POSIX sense, so detaching means two creation flags:
// CREATE_NO_WINDOW gives the child a console of its own that has no window,
// which keeps it off the parent's console (otherwise closing the console
// window would take the server with it, and a console would flash when
// `tars init` runs from Explorer), and CREATE_NEW_PROCESS_GROUP stops
// Ctrl-C/Ctrl-Break in the parent's console from being delivered to the child.
//
// It must not be DETACHED_PROCESS: that leaves the server with no console at
// all, and Windows then opens a new console window for every console program
// the server runs (the claude CLI, the exec tool's shell, git, gh). With a
// windowless console those programs inherit it and show nothing. Windows also
// ignores CREATE_NO_WINDOW when DETACHED_PROCESS is set, so the two cannot be
// combined.
func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
