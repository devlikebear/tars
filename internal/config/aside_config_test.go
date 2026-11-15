package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAsideFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tars.config.yaml")
	yaml := "tools:\n  web_search:\n    provider: aside\n    aside:\n      engine: Naver\n  aside:\n    path: /opt/aside/bin/aside\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolsWebSearchProvider != "aside" || cfg.ToolsWebSearchAsideEngine != "naver" || cfg.ToolsAsidePath != "/opt/aside/bin/aside" {
		t.Fatalf("got provider=%q engine=%q path=%q", cfg.ToolsWebSearchProvider, cfg.ToolsWebSearchAsideEngine, cfg.ToolsAsidePath)
	}
	if got := preferredYAMLPathForKey("tools_aside_path"); got != "tools.aside.path" {
		t.Fatalf("yaml path %q", got)
	}
}

func TestAsideEngineDefault(t *testing.T) {
	var cfg Config
	applyDefaults(&cfg)
	if cfg.ToolsWebSearchAsideEngine != "google" {
		t.Fatalf("got %q", cfg.ToolsWebSearchAsideEngine)
	}
}
