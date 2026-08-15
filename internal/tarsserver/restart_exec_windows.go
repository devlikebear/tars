//go:build windows

package tarsserver

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// execRestart starts a successor process and exits.
//
// Windows has no execve: syscall.Exec exists only as a stub that always returns
// EWINDOWS, so the unix path would leave the server dead. Spawning instead
// means predecessor and successor are briefly alive at the same time and the
// old process still owns the API port, so the successor is told to wait for the
// port before binding (see restartPredecessorEnv).
//
// This function does not return on success — the process exits so the port is
// released as soon as possible.
func execRestart(exe string, args []string, env []string) error {
	if len(args) == 0 {
		return fmt.Errorf("restart requires at least the program name in args")
	}

	cmd := exec.Command(exe, args[1:]...)
	cmd.Env = append(append([]string{}, env...), restartPredecessorEnv+"="+fmt.Sprint(os.Getpid()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// A new process group keeps a Ctrl-C in the old console from reaching the
	// successor before it has finished starting up.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
