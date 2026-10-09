package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPulseDeciderDefaultsAndOverrides(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseDecider != "rules" {
		t.Fatalf("pulse decider = %q, want rules by default", cfg.PulseDecider)
	}
	if cfg.LLMRoleDefaults["pulse_decider"] != "light" {
		t.Fatalf("pulse_decider role = %q, want light", cfg.LLMRoleDefaults["pulse_decider"])
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "automation:\n  pulse:\n    decider: LLM\nllm:\n  role_defaults:\n    pulse_decider: standard\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PulseDecider != "llm" {
		t.Fatalf("pulse decider = %q, want llm", cfg.PulseDecider)
	}
	if cfg.LLMRoleDefaults["pulse_decider"] != "standard" {
		t.Fatalf("an explicit role tier must win, got %q", cfg.LLMRoleDefaults["pulse_decider"])
	}
}
