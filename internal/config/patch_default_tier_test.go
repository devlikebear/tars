package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPatchDefaultTierValidation(t *testing.T) {
	const original = "log_level: info\nllm:\n  providers:\n    p:\n      kind: openai\n  tiers:\n    standard:\n      provider: p\n      model: some-model\n"
	const complete = original + "    heavy:\n      provider: p\n      model: some-model\n    light:\n      provider: p\n      model: some-model\n"
	for _, name := range []string{"missing", "heavy"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if err := PatchYAML(path, map[string]any{"llm_default_tier": name}); err == nil {
				t.Fatal("invalid default accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Fatal("rejected default modified file")
			}
		})
	}
	for _, name := range []string{"standard", " STANDARD ", ""} {
		t.Run("valid_"+name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(complete), 0600); err != nil {
				t.Fatal(err)
			}
			if err := PatchYAML(path, map[string]any{"llm_default_tier": name}); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LLMDefaultTier != "standard" {
				t.Fatalf("default = %q", cfg.LLMDefaultTier)
			}
		})
	}
}
