//go:build windows

package focusprobe

import (
	"os/exec"
	"time"
)

// KillProcessGroupOnCancel keeps exec's default cancel (kill git) on
// Windows; cmd.WaitDelay then closes the pipes a surviving child holds.
func KillProcessGroupOnCancel(_ *exec.Cmd, _ time.Duration) {}
