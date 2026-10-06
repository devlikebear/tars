package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchProviderAuthModeContract(t *testing.T) {
	for _, kind := range []string{"openai-codex", "openai", "anthropic", "gemini", "gemini-native", "kimi", "claude-code-cli", "antigravity-cli"} {
		for _, mode := range []string{"", "oauth", "api-key", " OAUTH ", "cli", "invalid"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.yaml")
				original := []byte("llm:\n  providers:\n    p: {kind: " + kind + "}\n  tiers:\n    heavy: {provider: p, model: m}\n    standard: {provider: p, model: m}\n    light: {provider: p, model: m}\n")
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{"p": map[string]any{"kind": kind, "auth_mode": mode}}})
				valid := mode != "cli" && mode != "invalid" || kind == "claude-code-cli" || kind == "antigravity-cli"
				if valid {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var validation *PatchValidationError
					if !errors.As(err, &validation) {
						t.Fatalf("expected validation error: %v", err)
					}
					after, readErr := os.ReadFile(path)
					if readErr != nil || string(after) != string(original) {
						t.Fatal("invalid mode changed original")
					}
				}
			})
		}
	}
}
