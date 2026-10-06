package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchNullProviderFieldsPreserveFile(t *testing.T) {
	for _, field := range []string{"api_key", "kind", "auth_mode", "base_url"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			original := []byte("llm:\n  providers:\n    p: {kind: openai, api_key: old-secret}\n  tiers:\n    heavy: {provider: p, model: m}\n    standard: {provider: p, model: m}\n    light: {provider: p, model: m}\n")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			provider := map[string]any{"kind": "openai", field: nil}
			err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"p": provider}})
			var invalid *PatchValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("expected validation error, got %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(original) {
				t.Fatal("invalid input changed original file")
			}
			cfg, err := LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LLMProviders["p"].APIKey != "old-secret" {
				t.Fatal("credential changed")
			}
		})
	}
}
