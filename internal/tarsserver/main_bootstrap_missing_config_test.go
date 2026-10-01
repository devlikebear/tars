package tarsserver

import (
	"os"
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

// A first `tars serve` without `tars init` used to boot with
// api_auth_mode=required and no tokens, so every wizard PATCH to
// /v1/admin/config/values was a 401 and setup could never finish.
func TestLoadConfigForServe_FirstRunOnLoopbackWritesSkeleton(t *testing.T) {
	t.Setenv("TARS_CONFIG", "")
	t.Setenv("TARS_CONFIG_PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	opts := &options{APIAddr: DefaultAPIAddr}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if opts.ConfigPath != config.FixedConfigPath() {
		t.Fatalf("config path = %q, want %q", opts.ConfigPath, config.FixedConfigPath())
	}
	if _, err := os.Stat(config.FixedConfigPath()); err != nil {
		t.Fatalf("skeleton not written: %v", err)
	}
	if !config.NeedsSetup(cfg) {
		t.Fatal("skeleton must still need setup")
	}
	if cfg.APIAuthMode != "off" || !cfg.APIAllowInsecureLocalAuth {
		t.Fatalf("auth = %q / insecure local %v, want off / true", cfg.APIAuthMode, cfg.APIAllowInsecureLocalAuth)
	}
	if err := validateAPIAuthSecurity(cfg); err != nil {
		t.Fatalf("skeleton auth posture rejected: %v", err)
	}
}

func TestLoadConfigForServe_FirstRunLeavesNonLoopbackAndExplicitPathsAlone(t *testing.T) {
	t.Setenv("TARS_CONFIG", "")
	t.Setenv("TARS_CONFIG_PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// Listening beyond loopback with auth off would open the server to the
	// network, so the skeleton is only written for loopback addresses.
	opts := &options{APIAddr: "0.0.0.0:43180"}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := os.Stat(config.FixedConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("non-loopback first run must not write a config, stat err = %v", err)
	}
	if cfg.APIAuthMode == "off" {
		t.Fatal("non-loopback first run must keep auth on")
	}

	// An explicit --config is the operator's file to write.
	explicit := filepath.Join(t.TempDir(), "mine.yaml")
	if _, err := loadConfigForServe(&options{ConfigPath: explicit, APIAddr: DefaultAPIAddr}); err != nil {
		t.Fatalf("load explicit: %v", err)
	}
	if _, err := os.Stat(explicit); !os.IsNotExist(err) {
		t.Fatalf("explicit missing config must not be written, stat err = %v", err)
	}
}

func TestLoadConfigForServe_FirstRunWithFixedPathFlagWritesSkeleton(t *testing.T) {
	t.Setenv("TARS_CONFIG", "")
	t.Setenv("TARS_CONFIG_PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// The LaunchAgent runs `tars serve --config ~/.tars/config/config.yaml`.
	cfg, err := loadConfigForServe(&options{ConfigPath: config.FixedConfigPath(), APIAddr: DefaultAPIAddr})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.APIAuthMode != "off" {
		t.Fatalf("auth = %q, want off from the skeleton", cfg.APIAuthMode)
	}
	// An existing file is never replaced.
	if err := os.WriteFile(config.FixedConfigPath(), []byte("api:\n  auth_mode: required\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigForServe(&options{ConfigPath: config.FixedConfigPath(), APIAddr: DefaultAPIAddr})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.APIAuthMode != "required" {
		t.Fatalf("existing config overwritten: auth = %q", cfg.APIAuthMode)
	}
}

func TestIsLoopbackListenAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:43180": true,
		"localhost:43180": true,
		"[::1]:43180":     true,
		"0.0.0.0:43180":   false,
		":43180":          false,
		"10.0.0.5:43180":  false,
		"":                false,
	} {
		if got := isLoopbackListenAddr(addr); got != want {
			t.Errorf("isLoopbackListenAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}
