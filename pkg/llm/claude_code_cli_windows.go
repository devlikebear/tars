//go:build windows

package llm

import (
	"os/exec"
	"syscall"
)

// claudeCodeCLICreateNoWindow is the Win32 CREATE_NO_WINDOW process creation
// flag, which package syscall does not name.
const claudeCodeCLICreateNoWindow = 0x08000000

// configureClaudeCodeCLIProcess bounds the invocation on Windows via WaitDelay.
// Windows has no POSIX process groups, so we rely on exec.CommandContext's
// default cancel (which terminates claude) plus WaitDelay: if a descendant
// (e.g. an stdio MCP server) outlives claude and keeps the stdout pipe open,
// WaitDelay makes os/exec force-close the pipe so the stream read unblocks at
// the deadline instead of hanging. Orphaned descendants then exit on their own
// once their stdio breaks.
//
// claude is a console program and all its I/O goes through pipes, so it gets a
// console with no window: a host with no console of its own, such as a desktop
// app, would otherwise put a terminal on screen for every invocation.
func configureClaudeCodeCLIProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = claudeCodeCLIWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: claudeCodeCLICreateNoWindow}
}
