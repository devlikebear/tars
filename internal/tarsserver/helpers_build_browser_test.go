package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestBuildBrowserPluginConfig_PopulatesFields(t *testing.T) {
	cfg := config.Default()
	cfg.BrowserRuntimeEnabled = true
	cfg.BrowserDefaultProfile = "chrome"
	cfg.WorkspaceDir = t.TempDir()

	m := buildBrowserPluginConfig(cfg, nil, vaultStatusSnapshot{}, nil)
	if enabled, _ := m["browser_runtime_enabled"].(bool); !enabled {
		t.Fatal("expected browser_runtime_enabled=true")
	}
	if profile, _ := m["browser_default_profile"].(string); profile != "chrome" {
		t.Fatalf("expected browser_default_profile=chrome, got %q", profile)
	}
}
