//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session, so it outlives the shell and does
// not get the shell's terminal signals.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// hideWindow is a no-op: only Windows opens a console for a child process.
func hideWindow(*exec.Cmd) {}
