package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand_InitCreatesStarterWorkspace(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "starter-workspace")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}
	configPath := filepath.Join(workspaceAbs, "config", "tars.config.yaml")
	assertPathExists(t, configPath)
	assertPathExists(t, filepath.Join(workspaceAbs, "memory"))
	assertPathExists(t, filepath.Join(workspaceAbs, "MEMORY.md"))
	assertPathExists(t, filepath.Join(workspaceAbs, "AGENTS.md"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := string(data)
	if !strings.Contains(configText, "workspace_dir: "+workspaceAbs) {
		t.Fatalf("expected workspace_dir %q in config, got:\n%s", workspaceAbs, configText)
	}
	if !strings.Contains(configText, "api_auth_mode: off") {
		t.Fatalf("expected local starter auth mode in config, got:\n%s", configText)
	}
	if !strings.Contains(configText, "${OPENAI_API_KEY}") {
		t.Fatalf("expected OPENAI_API_KEY placeholder in config, got:\n%s", configText)
	}

	out := stdout.String()
	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Fatalf("expected BYOK guidance in output, got:\n%s", out)
	}
	if !strings.Contains(out, "tars serve --config") {
		t.Fatalf("expected next-step serve command in output, got:\n%s", out)
	}
}

func TestRootCommand_InitRefusesToOverwriteExistingConfig(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "starter-workspace")
	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}
	configPath := filepath.Join(workspaceAbs, "config", "tars.config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("sentinel-config"), 0o644); err != nil {
		t.Fatalf("write sentinel config: %v", err)
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected init to fail when config already exists")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read sentinel config: %v", readErr)
	}
	if string(data) != "sentinel-config" {
		t.Fatalf("expected existing config to stay unchanged, got %q", string(data))
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %q to exist: %v", path, err)
	}
}
