package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchRoleDefaultsValidation(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, tc := range []struct {
			name, role, tier string
			valid            bool
		}{
			{"valid", "chat_main", "heavy", true},
			{"runtime normalization", " CHAT_MAIN ", " HEAVY ", true},
			{"unknown role", "chat_mian", "heavy", false},
			{"unknown tier", "chat_main", "haevy", false},
			{"empty role", "", "heavy", false},
			{"empty tier", "chat_main", "", false},
		} {
			label := "partial/"
			if configured {
				label = "configured/"
			}
			t.Run(label+tc.name, func(t *testing.T) {
				original := []byte("log_level: info\n")
				if configured {
					original = append(original, []byte("llm:\n  providers:\n    p: {kind: openai, api_key: test-secret}\n  tiers:\n    heavy: {provider: p, model: m}\n    standard: {provider: p, model: m}\n    light: {provider: p, model: m}\n  default_tier: standard\n")...)
				}
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				err := PatchYAML(path, map[string]any{"llm_role_defaults": map[string]any{tc.role: tc.tier}})
				if tc.valid {
					if err != nil {
						t.Fatal(err)
					}
					cfg, err := LoadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if cfg.LLMRoleDefaults[strings.ToLower(strings.TrimSpace(tc.role))] != strings.ToLower(strings.TrimSpace(tc.tier)) {
						t.Fatalf("role setting not saved: %v", cfg.LLMRoleDefaults)
					}
				} else {
					var validation *PatchValidationError
					if !errors.As(err, &validation) {
						t.Fatalf("expected validation error, got %v", err)
					}
					saved, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if string(saved) != string(original) {
						t.Fatal("invalid role setting changed original file")
					}
				}
			})
		}
	}
}
