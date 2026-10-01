package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleDefaultMode(t *testing.T) {
	t.Run("default is empty (advanced)", func(t *testing.T) {
		if got := defaultConfigValues().Console.DefaultMode; got != "" {
			t.Fatalf("default mode = %q, want empty", got)
		}
	})

	tests := []struct {
		raw  string
		want string
	}{
		{"focus", "focus"},
		{"  Focus ", "focus"},
		{"advanced", "advanced"},
		{"ADVANCED", "advanced"},
		{"simple", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run("yaml "+tt.raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("console:\n  default_mode: \""+tt.raw+"\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Console.DefaultMode != tt.want {
				t.Fatalf("mode = %q, want %q", cfg.Console.DefaultMode, tt.want)
			}
		})
	}

	t.Run("env", func(t *testing.T) {
		t.Setenv("TARS_CONSOLE_DEFAULT_MODE", "focus")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Console.DefaultMode != "focus" {
			t.Fatalf("mode = %q", cfg.Console.DefaultMode)
		}
	})
}

func TestConfigSchemaIncludesConsoleDefaultMode(t *testing.T) {
	for _, field := range Schema() {
		if field.Key != "console_default_mode" {
			continue
		}
		if field.Section != "Console" || field.Path != "console.default_mode" || field.Type != "select" {
			t.Fatalf("field = %+v", field)
		}
		if len(field.Options) != 2 || field.Options[0] != "advanced" || field.Options[1] != "focus" {
			t.Fatalf("options = %v", field.Options)
		}
		cfg := defaultConfigValues()
		cfg.Console.DefaultMode = "focus"
		if got := extractValue("console_default_mode", cfg); got != "focus" {
			t.Fatalf("extractValue = %v", got)
		}
		return
	}
	t.Fatal("expected console_default_mode schema field")
}
