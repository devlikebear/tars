package tarsserver

import (
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestLoadConfigForServe_MissingFileFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.yaml")

	opts := &options{ConfigPath: missing}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("expected nil error on missing config (setup-only fallback), got %v", err)
	}
	// The default config carries no LLM bindings — confirms downstream
	// buildLLMDeps will trigger the recoverable downgrade path.
	if !config.NeedsSetup(cfg) {
		t.Fatalf("expected NeedsSetup=true on default cfg, got false")
	}
	// opts.ConfigPath is preserved so handlers can advertise / save here.
	if opts.ConfigPath != missing {
		t.Fatalf("expected opts.ConfigPath preserved, got %q", opts.ConfigPath)
	}
}

func TestLoadConfigForServe_EmptyConfigPathFallsBackToFixedPath(t *testing.T) {
	// Simulate "no --config / TARS_CONFIG / DefaultConfigFilename" by
	// passing empty ConfigPath and pointing HOME at a fresh tempdir
	// so FixedConfigPath() resolves to a non-existent location. The
	// loadConfigForServe contract: when nothing is found on disk,
	// fill opts.ConfigPath with FixedConfigPath() so the wizard PATCH
	// has somewhere concrete to land.
	t.Setenv("TARS_CONFIG", "")
	t.Setenv("TARS_CONFIG_PATH", "")
	t.Setenv("HOME", t.TempDir())

	opts := &options{ConfigPath: ""}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("expected nil error on empty path, got %v", err)
	}
	if !config.NeedsSetup(cfg) {
		t.Fatalf("expected NeedsSetup=true on default cfg")
	}
	if opts.ConfigPath == "" {
		t.Fatalf("expected opts.ConfigPath to be filled with a fallback, got empty")
	}
	// FixedConfigPath() should reflect the overridden HOME.
	if got := opts.ConfigPath; got != config.FixedConfigPath() {
		t.Fatalf("expected fallback to FixedConfigPath() %q, got %q", config.FixedConfigPath(), got)
	}
}

func TestLoadConfigForServe_WorkspaceDirOverrideAppliesEvenWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "config.yaml")
	override := filepath.Join(dir, "ws-override")

	opts := &options{ConfigPath: missing, WorkspaceDir: override}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("loadConfigForServe missing+override: %v", err)
	}
	if cfg.WorkspaceDir != override {
		t.Fatalf("expected workspace_dir override %q, got %q", override, cfg.WorkspaceDir)
	}
}

func TestLoadConfigForServe_MissingFileAppliesEnvOverrides(t *testing.T) {
	// Regression for #650: when the config file does not exist yet,
	// the fall-through must still run applyEnv so TARS_API_AUTH_MODE
	// (and friends) reach the cfg the middleware sees. Without this
	// the wizard's first PATCH /v1/admin/config/values returns 401.
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.yaml")

	t.Setenv("TARS_API_AUTH_MODE", "off")
	t.Setenv("TARS_API_ALLOW_INSECURE_LOCAL_AUTH", "true")
	t.Setenv("TARS_DASHBOARD_AUTH_MODE", "off")

	opts := &options{ConfigPath: missing}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("loadConfigForServe with envs: %v", err)
	}
	if cfg.APIAuthMode != "off" {
		t.Fatalf("expected APIAuthMode=off from env, got %q", cfg.APIAuthMode)
	}
	if !cfg.APIAllowInsecureLocalAuth {
		t.Fatalf("expected APIAllowInsecureLocalAuth=true from env, got false")
	}
	if cfg.DashboardAuthMode != "off" {
		t.Fatalf("expected DashboardAuthMode=off from env, got %q", cfg.DashboardAuthMode)
	}
}
