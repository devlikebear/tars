//go:build windows

package computeruse

import "os/exec"

// configureCuaDriverProcess has no process-group handling on Windows; the wait
// delay alone keeps a wedged helper from holding the pipe open after cancel.
func configureCuaDriverProcess(cmd *exec.Cmd) { cmd.WaitDelay = cuaDriverWaitDelay }
