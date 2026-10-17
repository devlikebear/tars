package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Keys removed in #1240 must not break a config file that still has them.
func TestLoadIgnoresRemovedConfigKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tars.config.yaml")
	body := "memory:\n  backend: sqlite\nautomation:\n  schedule:\n    timezone: UTC\nassistant:\n  enabled: true\n  hotkey: Ctrl+Space\nlog_level: warn\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want warn", cfg.LogLevel)
	}
	for _, key := range []string{"memory_backend", "schedule_timezone", "assistant_enabled", "assistant_hotkey"} {
		if _, ok := configInputFieldByYAMLKey(key); ok {
			t.Errorf("%s is still a config field", key)
		}
	}
}
