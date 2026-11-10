package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExecSandboxIsOffByDefault(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolsExecSandboxEnabled || cfg.ToolsExecSandboxNetwork || len(cfg.ToolsExecSandboxWritablePaths) != 0 {
		t.Fatalf("sandbox must be off by default: %+v", cfg.ToolConfig)
	}
}

func TestExecSandboxLoadsFromYAMLAndEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "tools:\n  exec:\n    sandbox:\n      enabled: true\n      network: true\n      writable_paths: [~/.cache/pip, /opt/build]\n      read_only_paths: [/opt/build/keep]\n      deny_read_paths: [~/.secrets]\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ToolsExecSandboxEnabled || !cfg.ToolsExecSandboxNetwork {
		t.Fatalf("enabled/network not loaded: %v %v", cfg.ToolsExecSandboxEnabled, cfg.ToolsExecSandboxNetwork)
	}
	if want := []string{"~/.cache/pip", "/opt/build"}; !reflect.DeepEqual(cfg.ToolsExecSandboxWritablePaths, want) {
		t.Fatalf("writable = %v, want %v", cfg.ToolsExecSandboxWritablePaths, want)
	}
	if want := []string{"/opt/build/keep"}; !reflect.DeepEqual(cfg.ToolsExecSandboxReadOnlyPaths, want) {
		t.Fatalf("read only = %v, want %v", cfg.ToolsExecSandboxReadOnlyPaths, want)
	}
	if want := []string{"~/.secrets"}; !reflect.DeepEqual(cfg.ToolsExecSandboxDenyReadPaths, want) {
		t.Fatalf("deny read = %v, want %v", cfg.ToolsExecSandboxDenyReadPaths, want)
	}

	t.Setenv("TARS_TOOLS_EXEC_SANDBOX_ENABLED", "true")
	cfg, err = Load("")
	if err != nil || !cfg.ToolsExecSandboxEnabled {
		t.Fatalf("env: %v %v", cfg.ToolsExecSandboxEnabled, err)
	}
}
