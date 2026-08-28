//go:build windows

package proofverifier

import "os/exec"

// configureCommand bounds the verification shell on Windows via WaitDelay.
// Windows has no POSIX process groups, so we rely on exec.CommandContext's
// default cancel (which terminates the shell) plus WaitDelay: if a descendant
// spawned by the verified command outlives the shell and keeps the stdout pipe
// open, WaitDelay makes os/exec force-close the pipe so Run returns at the
// deadline instead of hanging. Orphaned descendants then exit on their own once
// their stdio breaks.
func configureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
}
