package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPatchClearProviderEndpointPreservesCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "llm:\n  providers:\n    p:\n      kind: openai\n      base_url: https://old-gateway.example/v1\n      api_key: existing-secret\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"p": map[string]any{"kind": "openai", "base_url": ""}}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMProviders["p"].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("endpoint was not cleared: %#v", cfg.LLMProviders["p"])
	}
	if cfg.LLMProviders["p"].APIKey != "existing-secret" {
		t.Fatal("credential was not preserved")
	}
}
