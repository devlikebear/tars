package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFocusHarnessRoleDefaultsToHeavy(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.LLMRoleDefaults["focus_harness"]; got != "heavy" {
		t.Fatalf("focus_harness role = %q", got)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("llm:\n  role_defaults:\n    focus_harness: standard\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(path); err != nil {
		t.Fatal(err)
	}
	if got := cfg.LLMRoleDefaults["focus_harness"]; got != "standard" {
		t.Fatalf("mapped focus_harness role = %q", got)
	}
}
