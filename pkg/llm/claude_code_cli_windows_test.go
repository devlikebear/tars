//go:build windows

package llm

import (
	"os/exec"
	"testing"
)

func TestConfigureClaudeCodeCLIProcess_opensNoConsoleWindow(t *testing.T) {
	cmd := exec.Command("claude")
	configureClaudeCodeCLIProcess(cmd)

	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&claudeCodeCLICreateNoWindow == 0 {
		t.Fatalf("CREATE_NO_WINDOW not set: %+v", cmd.SysProcAttr)
	}
	if cmd.WaitDelay != claudeCodeCLIWaitDelay {
		t.Errorf("WaitDelay = %s, want %s", cmd.WaitDelay, claudeCodeCLIWaitDelay)
	}
}
