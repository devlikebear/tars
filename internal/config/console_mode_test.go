package config

import (
	"os"
	"path/filepath"
	"strings"
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
		{"easy", "easy"},
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
		if strings.Join(field.Options, ",") != "advanced,focus,easy" {
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

func TestConsoleExpertMode(t *testing.T) {
	if defaultConfigValues().Console.ExpertMode {
		t.Fatal("expert mode on by default, want off")
	}
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"console:\n  expert_mode: true\n", true},
		{"console:\n  expert_mode: false\n", false},
		{"console:\n  default_mode: advanced\n", false},
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Console.ExpertMode != tt.want {
			t.Fatalf("%q: expert mode = %v, want %v", tt.yaml, cfg.Console.ExpertMode, tt.want)
		}
	}
	t.Setenv("TARS_CONSOLE_EXPERT_MODE", "true")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Console.ExpertMode {
		t.Fatal("env did not turn expert mode on")
	}
	if got := extractValue("console_expert_mode", cfg); got != true {
		t.Fatalf("schema value = %v", got)
	}
}
