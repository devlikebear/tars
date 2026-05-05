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
	// --no-server / --no-browser keeps this test focused on file
	// scaffolding; orchestration behavior is covered by the dedicated
	// orchestrator tests below.
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})

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
	assertPathNotExists(t, filepath.Join(workspaceAbs, "memory", "wiki"))
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
	if !strings.Contains(configText, "api:\n  auth_mode: off") {
		t.Fatalf("expected local starter auth mode in config, got:\n%s", configText)
	}
	if !strings.Contains(configText, "agentruntime:\n  enabled: true") {
		t.Fatalf("expected starter agent runtime to be enabled, got:\n%s", configText)
	}
	// Skeleton intentionally has no llm_providers — that triggers the
	// setup wizard the orchestrator opens in the browser.
	if strings.Contains(configText, "\nllm_providers:") || strings.Contains(configText, "\nproviders:") {
		t.Fatalf("expected no llm providers in skeleton config, got:\n%s", configText)
	}

	out := stdout.String()
	if !strings.Contains(out, "api addr:") {
		t.Fatalf("expected api addr in output, got:\n%s", out)
	}
	if !strings.Contains(out, "skipped server start") {
		t.Fatalf("expected skipped-server hint in output, got:\n%s", out)
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
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected init to fail when config already exists")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "--force") {
		t.Fatalf("expected --force hint in error, got %v", err)
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
	// Migration must not require the bundled plugins dir — the
	// migrated workspace already has its own layout. Setting an empty
	// value forces the resolver to fail if init tries to scaffold.
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", filepath.Join(t.TempDir(), "no-such-bundled-plugins"))

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
	cmd.SetArgs([]string{"init", "--migrate", "--no-server", "--no-browser"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	// Fixed config should exist with migrated content.
	fixedPath := config.FixedConfigPath()
	assertPathExists(t, fixedPath)
	cfg, err := config.Load(fixedPath)
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	if !filepath.IsAbs(cfg.WorkspaceDir) {
		t.Fatalf("expected migrated workspace_dir to be absolute, got %q", cfg.WorkspaceDir)
	}

	out := stdout.String()
	if !strings.Contains(out, "migrated legacy config") {
		t.Fatalf("expected migration output, got:\n%s", out)
	}
	// Migration must NOT overwrite the migrated payload with the
	// wizard skeleton — that would silently destroy LLM creds and
	// other settings the user already had.
	migrated, err := os.ReadFile(fixedPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(migrated), "mode: standalone") {
		t.Fatalf("expected migrated payload (mode: standalone) preserved, got:\n%s", string(migrated))
	}
	if strings.Contains(string(migrated), "TARS skeleton config generated by") {
		t.Fatalf("migration must not overwrite with skeleton, got:\n%s", string(migrated))
	}

	// Original should still exist.
	assertPathExists(t, legacyConfig)
}

func TestUpdateMigratedWorkspaceDir_PatchesNestedRuntimeKey(t *testing.T) {
	// Regression: most legacy configs put workspace_dir under a
	// `runtime:` block. The patcher used to look only at the top-level
	// key, so a nested relative path went unfixed AND the function
	// added a stray top-level entry pointing at the default
	// (~/.tars/workspace). Both keys flatten to the same field, so
	// the result was a coin-flip between the user's real workspace
	// and an empty default.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	src := "runtime:\n    workspace_dir: ./workspace\nmode: standalone\n"
	if err := os.WriteFile(configPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := updateMigratedWorkspaceDir(configPath, "/should/not/be/used"); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cwd, _ := os.Getwd()
	wantAbs, _ := filepath.Abs(filepath.Join(cwd, "workspace"))
	if !strings.Contains(string(out), "workspace_dir: "+wantAbs) {
		t.Fatalf("expected nested workspace_dir absolutized to %q, got:\n%s", wantAbs, string(out))
	}
	// Must NOT add a stray top-level entry.
	if strings.Contains(string(out), "\nworkspace_dir: ") {
		// Allow runtime nesting (4-space indent before the colon) by
		// being strict about no top-level entry.
		t.Fatalf("expected no top-level workspace_dir, got:\n%s", string(out))
	}
}

func TestUpdateMigratedWorkspaceDir_PatchesTopLevelKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	src := "workspace_dir: ./workspace\nmode: standalone\n"
	if err := os.WriteFile(configPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := updateMigratedWorkspaceDir(configPath, "/should/not/be/used"); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cwd, _ := os.Getwd()
	wantAbs, _ := filepath.Abs(filepath.Join(cwd, "workspace"))
	if !strings.Contains(string(out), "workspace_dir: "+wantAbs) {
		t.Fatalf("expected top-level workspace_dir absolutized to %q, got:\n%s", wantAbs, string(out))
	}
}

func TestUpdateMigratedWorkspaceDir_NoOpWhenAlreadyAbsolute(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	src := "runtime:\n    workspace_dir: /already/absolute\n"
	if err := os.WriteFile(configPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := updateMigratedWorkspaceDir(configPath, "/should/not/be/used"); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, _ := os.ReadFile(configPath)
	if string(out) != src {
		t.Fatalf("expected file unchanged, got:\n%s", string(out))
	}
}

func TestUpdateMigratedWorkspaceDir_AddsDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("mode: standalone\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := updateMigratedWorkspaceDir(configPath, "/default/workspace"); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, _ := os.ReadFile(configPath)
	if !strings.Contains(string(out), "workspace_dir: /default/workspace") {
		t.Fatalf("expected default workspace_dir added, got:\n%s", string(out))
	}
}

func TestUpdateMigratedWorkspaceDirReportsWriteFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission-based write failure is not reliable as root")
	}
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("workspace_dir: ./workspace\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chmod(configPath, 0o400); err != nil {
		t.Fatalf("chmod config: %v", err)
	}
	defer func() { _ = os.Chmod(configPath, 0o600) }()

	if err := updateMigratedWorkspaceDir(configPath, "/tmp/tars-workspace"); err == nil {
		t.Fatal("expected workspace_dir update write failure")
	}
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
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
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

func assertPathNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %q not to exist, got err=%v", path, err)
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
