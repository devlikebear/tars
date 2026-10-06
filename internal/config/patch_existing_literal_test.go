package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingLiteralProviderKeyPreservation(t *testing.T) {
	for _, format := range []string{"flat", "nested", "mixed"} {
		for _, masked := range []bool{false, true} {
			t.Run(format+map[bool]string{false: "/omitted", true: "/masked"}[masked], func(t *testing.T) {
				flat := "llm_providers:\n  p: {kind: claude-code-cli, api_key: literal-test-key}\n"
				nested := "llm:\n  providers:\n    p: {kind: claude-code-cli, api_key: literal-test-key}\n"
				original := flat
				if format == "nested" {
					original = nested
				}
				if format == "mixed" {
					original = flat + strings.ReplaceAll(nested, "literal-test-key", "wrong-nested-key")
				}
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				fields := map[string]any{"kind": "claude-code-cli", "auth_mode": "cli", "base_url": ""}
				if masked {
					fields["api_key"] = "****"
				}
				if err := PatchYAML(path, map[string]any{"llm_providers": map[string]any{" p ": fields}}); err != nil {
					t.Fatal(err)
				}
				cfg, err := LoadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.LLMProviders["p"].APIKey != "literal-test-key" {
					t.Fatal("lost original credential")
				}
			})
		}
	}
}
