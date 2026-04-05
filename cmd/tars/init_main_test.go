package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestRootCommand_InitCreatesStarterWorkspace(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	bundledPluginsDir := writeBundledPluginSource(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", bundledPluginsDir)

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
	configPath := config.FixedConfigPath()
	assertPathExists(t, configPath)
	assertPathExists(t, filepath.Join(workspaceAbs, "memory"))
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "raw"))
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "wiki"))
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "wiki", "notes"))
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "wiki", "index.md"))
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "wiki", "graph.json"))
	assertPathExists(t, filepath.Join(workspaceAbs, "MEMORY.md"))
	assertPathExists(t, filepath.Join(workspaceAbs, "AGENTS.md"))
	assertPathExists(t, filepath.Join(workspaceAbs, "plugins", "ops-service", "tars.plugin.json"))

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
	if !strings.Contains(configText, "gateway_enabled: true") {
		t.Fatalf("expected starter gateway to be enabled, got:\n%s", configText)
	}
	if !strings.Contains(configText, "${OPENAI_API_KEY}") {
		t.Fatalf("expected OPENAI_API_KEY placeholder in config, got:\n%s", configText)
	}

	out := stdout.String()
	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Fatalf("expected BYOK guidance in output, got:\n%s", out)
	}
	if !strings.Contains(out, "tars serve") {
		t.Fatalf("expected next-step serve command in output, got:\n%s", out)
	}
}

func TestRootCommand_InitRefusesToOverwriteExistingConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	configPath := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("sentinel-config"), 0o644); err != nil {
		t.Fatalf("write sentinel config: %v", err)
	}

	workspaceDir := filepath.Join(t.TempDir(), "starter-workspace")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})

	err := cmd.Execute()
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

func TestRootCommand_InitMigratesLegacyConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Create legacy config in CWD.
	legacyDir := t.TempDir()
	legacyConfigDir := filepath.Join(legacyDir, "workspace", "config")
	if err := os.MkdirAll(legacyConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy config dir: %v", err)
	}
	legacyConfig := filepath.Join(legacyConfigDir, "tars.config.yaml")
	if err := os.WriteFile(legacyConfig, []byte("mode: standalone\nworkspace_dir: ./workspace\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	// Change to the directory with legacy config.
	wd, _ := os.Getwd()
	_ = os.Chdir(legacyDir)
	defer func() { _ = os.Chdir(wd) }()

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	// Fixed config should exist with migrated content.
	fixedPath := config.FixedConfigPath()
	assertPathExists(t, fixedPath)

	out := stdout.String()
	if !strings.Contains(out, "migrated legacy config") {
		t.Fatalf("expected migration output, got:\n%s", out)
	}

	// Original should still exist.
	assertPathExists(t, legacyConfig)
}

func TestRootCommand_InitMoveWorkspace(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	bundledPluginsDir := writeBundledPluginSource(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", bundledPluginsDir)

	// First init.
	workspaceDir := filepath.Join(t.TempDir(), "orig-workspace")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	workspaceAbs, _ := filepath.Abs(workspaceDir)
	assertPathExists(t, workspaceAbs)

	// Move workspace.
	targetDir := filepath.Join(t.TempDir(), "new-workspace")
	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "move", "--to", targetDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init move command: %v", err)
	}

	// Old workspace should be gone, new one should exist.
	if _, err := os.Stat(workspaceAbs); !os.IsNotExist(err) {
		t.Fatalf("expected old workspace to be removed, got err=%v", err)
	}
	targetAbs, _ := filepath.Abs(targetDir)
	assertPathExists(t, targetAbs)

	// Config should point to new workspace.
	cfg, err := config.Load(config.FixedConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WorkspaceDir != targetAbs {
		t.Fatalf("expected workspace_dir=%q, got %q", targetAbs, cfg.WorkspaceDir)
	}

	out := stdout.String()
	if !strings.Contains(out, "workspace moved") {
		t.Fatalf("expected move output, got:\n%s", out)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %q to exist: %v", path, err)
	}
}

func writeBundledPluginSource(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "bundled-plugins")

	opsPluginDir := filepath.Join(root, "ops-service")
	opsSkillDir := filepath.Join(opsPluginDir, "skills", "ops-plan")
	if err := os.MkdirAll(opsSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir ops plugin skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opsPluginDir, "tars.plugin.json"), []byte(`{
  "id": "ops-service",
  "name": "Ops Service",
  "version": "0.0.0-test",
  "skills": ["skills/ops-plan"]
}`), 0o644); err != nil {
		t.Fatalf("write ops plugin manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opsSkillDir, "SKILL.md"), []byte(`# Ops Plan`), 0o644); err != nil {
		t.Fatalf("write ops plugin skill: %v", err)
	}
	return root
}
