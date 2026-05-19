package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompanionConfig(t *testing.T) {
	t.Run("default is enabled", func(t *testing.T) {
		cfg := defaultConfigValues()
		if !cfg.Companion.Enabled {
			t.Fatalf("companion should be enabled by default: %+v", cfg.Companion)
		}
	})

	t.Run("yaml and env toggle companion", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(`
companion:
  enabled: false
`), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Companion.Enabled {
			t.Fatal("expected companion disabled from yaml")
		}

		t.Setenv("TARS_COMPANION_ENABLED", "false")
		cfg, err = Load("")
		if err != nil {
			t.Fatalf("Load env: %v", err)
		}
		if cfg.Companion.Enabled {
			t.Fatal("expected companion disabled from env")
		}
	})
}

func TestConfigSchemaIncludesCompanion(t *testing.T) {
	fields := Schema()
	var found bool
	for _, field := range fields {
		if field.Key != "companion_enabled" {
			continue
		}
		found = true
		if field.Section != "Companion" {
			t.Fatalf("section = %q, want Companion", field.Section)
		}
		if field.Path != "companion.enabled" {
			t.Fatalf("path = %q, want companion.enabled", field.Path)
		}
		if field.Type != "bool" {
			t.Fatalf("type = %q, want bool", field.Type)
		}
	}
	if !found {
		t.Fatal("expected companion_enabled schema field")
	}
}
