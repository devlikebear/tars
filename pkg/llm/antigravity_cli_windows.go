//go:build windows

package llm

import "os/exec"

// configureAntigravityCLIProcess bounds the invocation on Windows via WaitDelay.
// Windows has no POSIX process groups, so we rely on exec.CommandContext's
// default cancel (which terminates agy) plus WaitDelay: if a descendant
// (e.g. an stdio MCP server) outlives agy and keeps the stdout pipe open,
// WaitDelay makes os/exec force-close the pipe so the stream read unblocks at
// the deadline instead of hanging. Orphaned descendants then exit on their own
// once their stdio breaks.
func configureAntigravityCLIProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = antigravityCLIWaitDelay
}
