package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompleteProviderEdit(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, credential := range []string{"omitted", "masked", "new"} {
			for _, alias := range []string{"p", " p "} {
				t.Run(alias+credential+map[bool]string{false: "deleted", true: "empty"}[empty], func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "config.yaml")
					original := []byte("llm:\n  providers:\n    p:\n      kind: openai\n      auth_mode: oauth\n      base_url: https://old.example/v1\n      api_key: old-secret\n")
					original = append(original, []byte("  tiers:\n    heavy: {provider: p, model: m}\n    standard: {provider: p, model: m}\n    light: {provider: p, model: m}\n")...)
					if err := os.WriteFile(path, original, 0600); err != nil {
						t.Fatal(err)
					}
					fields := map[string]any{"kind": "openai"}
					if empty {
						fields["auth_mode"] = ""
						fields["base_url"] = ""
					}
					if credential == "masked" {
						fields["api_key"] = "ol******et"
					}
					if credential == "new" {
						fields["api_key"] = "new-secret"
					}
					if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{alias: fields}}); err != nil {
						t.Fatal(err)
					}
					cfg, err := LoadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					p := cfg.LLMProviders["p"]
					if p.AuthMode != "api-key" || p.BaseURL != "https://api.openai.com/v1" {
						t.Fatalf("optional fields not reset: %#v", p)
					}
					want := "old-secret"
					if credential == "new" {
						want = "new-secret"
					}
					if p.APIKey != want {
						t.Fatal("credential not preserved/replaced")
					}
					before, _ := os.ReadFile(path)
					if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"p": map[string]any{}}}); err == nil {
						t.Fatal("missing kind accepted")
					}
					after, _ := os.ReadFile(path)
					if string(after) != string(before) {
						t.Fatal("invalid edit changed file")
					}
				})
			}
		}
	}
}
