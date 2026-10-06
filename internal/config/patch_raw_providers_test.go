package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawProviderPersistence(t *testing.T) {
	t.Setenv("PATCH_TEST_SECRET", "expanded-secret")
	flat := "llm_providers:\n  p: {kind: openai, auth_mode: oauth, base_url: 'https://old.example/v1', api_key: '${PATCH_TEST_SECRET}'}\n"
	nested := "llm:\n  providers:\n    p: {kind: openai, auth_mode: oauth, base_url: 'https://old.example/v1', api_key: '${PATCH_TEST_SECRET}'}\n"
	mixed := flat + "llm:\n  providers:\n    p: {kind: openai, api_key: wrong-nested-secret}\n"
	for format, source := range map[string]string{"flat": flat, "nested": nested, "mixed": mixed} {
		for _, secret := range []string{"omitted", "****", "new-secret", "${PATCH_TEST_SECRET}"} {
			t.Run(format+secret, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				fields := map[string]any{"kind": "openai"}
				if secret != "omitted" {
					fields["api_key"] = secret
				}
				if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{" p ": fields}}); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(raw)
				expected := secret
				if secret == "omitted" || secret == "****" {
					expected = "${PATCH_TEST_SECRET}"
				}
				if !strings.Contains(text, expected) || strings.Contains(text, "expanded-secret") || strings.Contains(text, "wrong-nested-secret") {
					t.Fatalf("wrong raw credential: %s", text)
				}
				cfg, err := LoadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				expectedRuntime := expected
				if expected == "${PATCH_TEST_SECRET}" {
					expectedRuntime = "expanded-secret"
				}
				if cfg.LLMProviders["p"].APIKey != expectedRuntime || cfg.LLMProviders["p"].AuthMode != "api-key" || cfg.LLMProviders["p"].BaseURL != "https://api.openai.com/v1" {
					t.Fatal("incorrect runtime provider")
				}
			})
		}
	}
	for _, oldCollision := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		source := flat
		if oldCollision {
			source = "llm_providers:\n  p: {kind: openai, api_key: first}\n  ' p ': {kind: openai, api_key: second}\n"
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		submitted := map[string]any{"p": map[string]any{"kind": "openai"}}
		if !oldCollision {
			submitted[" p "] = map[string]any{"kind": "openai", "api_key": "new"}
		}
		if err := PatchYAML(path, map[string]any{"llm_providers": submitted}); err == nil {
			t.Fatal("accepted alias collision")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != source {
			t.Fatal("modified rejected file")
		}
	}
}
