//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detach starts cmd without a console window and outside the shell's
// process group, so it outlives the shell.
//
// CREATE_NO_WINDOW gives the server a console that has no window. It must
// not be DETACHED_PROCESS (which Windows also lets override
// CREATE_NO_WINDOW): a server with no console at all makes Windows open a
// new console window for every console program it runs — the claude CLI,
// the exec tool's shell, git, gh — so a terminal flashed on every turn.
// With a windowless console those programs inherit it and show nothing.
// internal/onboarding's configureDetachedProcess has the same rule and the
// test for it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
}

// hideWindow keeps a console program the shell waits on, such as
// `tars update`, from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
