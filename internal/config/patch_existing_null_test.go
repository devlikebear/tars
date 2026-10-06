package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingNullCLIProviderEdit(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		fields := "{kind: claude-code-cli, api_key: null, auth_mode: null, base_url: null}"
		if omitted {
			fields = "{kind: claude-code-cli}"
		}
		flat := "llm_providers:\n  ' resettest ': " + fields + "\n"
		nested := "llm:\n  providers:\n    ' resettest ': " + fields + "\n"
		mixed := flat + "llm:\n  providers:\n    resettest: {kind: claude-code-cli, api_key: wrong-nested-secret}\n"
		for format, original := range map[string]string{"flat": flat, "nested": nested, "mixed": mixed} {
			for _, masked := range []bool{false, true} {
				t.Run(format+map[bool]string{false: "/null", true: "/omitted"}[omitted]+map[bool]string{false: "/no-key", true: "/masked"}[masked], func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "config.yaml")
					if err := os.WriteFile(path, []byte(original), 0600); err != nil {
						t.Fatal(err)
					}
					before, err := LoadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if before.LLMProviders["resettest"].APIKey != "" {
						t.Fatal("wrong original credential")
					}
					submitted := map[string]any{"kind": "claude-code-cli", "auth_mode": "cli", "base_url": ""}
					if masked {
						submitted["api_key"] = "****"
					}
					if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"resettest": submitted}}); err != nil {
						t.Fatal(err)
					}
					after, err := LoadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					p := after.LLMProviders["resettest"]
					if p.Kind != "claude-code-cli" || p.AuthMode != "cli" || p.APIKey != "" || p.BaseURL != "" {
						t.Fatalf("unexpected provider: %+v", p)
					}
				})
			}
		}
	}
}

func TestSubmittedProviderNonStringsPreserveOriginal(t *testing.T) {
	for _, field := range []string{"kind", "auth_mode", "base_url", "api_key"} {
		for name, value := range map[string]any{"null": nil, "number": 42, "bool": true} {
			t.Run(field+"/"+name, func(t *testing.T) {
				original := "llm_providers:\n  resettest: {kind: claude-code-cli, api_key: null}\n"
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				submitted := map[string]any{"kind": "claude-code-cli", field: value}
				err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"resettest": submitted}})
				var invalid *PatchValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("expected validation error: %v", err)
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != original {
					t.Fatal("rejected edit changed original")
				}
			})
		}
	}
}
