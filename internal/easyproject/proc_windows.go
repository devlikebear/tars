//go:build windows

package easyproject

import (
	"os/exec"
	"time"
)

// stopGroupOnCancel keeps exec's default cancel (kill the process) on
// Windows; cmd.WaitDelay then closes the pipes a surviving child holds.
func stopGroupOnCancel(_ *exec.Cmd, _ time.Duration) {}
