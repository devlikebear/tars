//go:build windows

package tools

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

func configureExecShellProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	cmd.WaitDelay = time.Second
}

func cleanupExecShellProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Cancel()
	}
}
