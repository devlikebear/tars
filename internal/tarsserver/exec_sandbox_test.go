package tarsserver

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestExecSandboxFromConfig(t *testing.T) {
	if got := execSandboxFromConfig(config.Config{}); got != nil {
		t.Fatalf("sandbox off must be nil, got %+v", got)
	}

	workspace := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
		ToolConfig: config.ToolConfig{
			ToolsExecSandboxEnabled:       true,
			ToolsExecSandboxNetwork:       true,
			ToolsExecSandboxWritablePaths: []string{"~/.cache/pip"},
			ToolsExecSandboxReadOnlyPaths: []string{"/opt/keep"},
			ToolsExecSandboxDenyReadPaths: []string{"~/.secrets"},
		},
	}
	got := execSandboxFromConfig(cfg)
	if got == nil || !got.Network {
		t.Fatalf("sandbox = %+v", got)
	}
	if !slices.Equal(got.WritablePaths, []string{"~/.cache/pip"}) {
		t.Fatalf("writable = %v", got.WritablePaths)
	}
	for _, want := range []string{"/opt/keep", filepath.Join(workspace, "_shared", "permissions")} {
		if !slices.Contains(got.ReadOnlyPaths, want) {
			t.Errorf("read-only lacks %s: %v", want, got.ReadOnlyPaths)
		}
	}
	for _, want := range []string{"~/.secrets", filepath.Join(workspace, "config"), filepath.Join(config.TarsHomeDir(), "config")} {
		if !slices.Contains(got.DenyReadPaths, want) {
			t.Errorf("hidden lacks %s: %v", want, got.DenyReadPaths)
		}
	}

	// TARS_CONFIG points at the config file the server loaded.
	configFile := filepath.Join(t.TempDir(), "tars.yaml")
	t.Setenv("TARS_CONFIG", configFile)
	if got := execSandboxFromConfig(cfg); !slices.Contains(got.DenyReadPaths, configFile) {
		t.Errorf("hidden lacks the loaded config file: %v", got.DenyReadPaths)
	}
}
