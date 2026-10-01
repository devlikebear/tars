//go:build windows

package tarsserver

import (
	"os/exec"
	"time"
)

// killProcessGroupOnCancel keeps exec's default cancel (kill git) on
// Windows; cmd.WaitDelay then closes the pipes a surviving child holds.
func killProcessGroupOnCancel(_ *exec.Cmd, _ time.Duration) {}
