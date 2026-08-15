//go:build !windows

package tarsserver

import "syscall"

// execRestart replaces the current process image with a fresh copy of the same
// binary. On success it never returns, so the listening socket is handed over
// atomically and there is nothing for the successor to wait on.
func execRestart(exe string, args []string, env []string) error {
	return syscall.Exec(exe, args, env)
}
