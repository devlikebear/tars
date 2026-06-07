//go:build windows

package llm

import (
	"os/exec"
	"strconv"
)

// configureClaudeCodeCLIProcess kills the whole claude descendant tree on
// context cancellation. Windows has no POSIX process groups, so instead of
// Setpgid+kill(-pgid) we issue `taskkill /T` to terminate claude and the
// descendants (e.g. stdio MCP servers) that would otherwise hold the stdout
// pipe open and block the stream read past the deadline. WaitDelay bounds any
// residual pipe wait.
func configureClaudeCodeCLIProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		return kill.Run()
	}
	cmd.WaitDelay = claudeCodeCLIWaitDelay
}
