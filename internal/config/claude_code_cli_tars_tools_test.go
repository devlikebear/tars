package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeCodeCLITarsToolsDefaultAndOverrides(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ClaudeCodeCLITarsTools {
		t.Fatal("TARS tools must be served to claude-code-cli turns by default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("llm:\n  claude_code_cli:\n    permission_mode: plan\n    tars_tools: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.ClaudeCodeCLITarsTools {
		t.Fatalf("an explicit false must win over the default: %v %v", cfg.ClaudeCodeCLITarsTools, err)
	}
	// The documented nested path was read by neither key before: the keys
	// have no llm_ prefix, so the path needs an alias.
	if cfg.ClaudeCodeCLIPermissionMode != "plan" {
		t.Fatalf("llm.claude_code_cli.permission_mode = %q, want plan", cfg.ClaudeCodeCLIPermissionMode)
	}
	t.Setenv("TARS_CLAUDE_CODE_CLI_TARS_TOOLS", "true")
	cfg, err = Load(path)
	if err != nil || !cfg.ClaudeCodeCLITarsTools {
		t.Fatalf("env override: %v %v", cfg.ClaudeCodeCLITarsTools, err)
	}
}
