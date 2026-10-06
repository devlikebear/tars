package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchReviewInvalidCandidatesPreserveOriginal(t *testing.T) {
	cases := []struct {
		name    string
		updates map[string]any
	}{
		{"missing provider", map[string]any{"llm_tiers": map[string]LLMTierBinding{"standard": {Provider: "missing", Model: "some-model"}}}},
		{"empty kind", map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {}}, "llm_tiers": map[string]LLMTierBinding{"standard": {Provider: "p", Model: "some-model"}}}},
		{"invalid budget", map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: "openai"}}, "llm_tiers": map[string]LLMTierBinding{"standard": {Provider: "p", Model: "some-model", MaxTokens: 100, ThinkingBudget: 100}}}},
		{"negative positive integer", map[string]any{"agent_max_iterations": -1}},
		{"zero positive integer", map[string]any{"agent_max_iterations": 0}},
		{"negative nonnegative integer", map[string]any{"usage_daily_token_budget": -1}},
		{"negative float", map[string]any{"usage_limit_daily_usd": -1.0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			original := "log_level: info\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if err := PatchYAML(path, tc.updates); err == nil {
				t.Fatal("invalid candidate accepted")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != original {
				t.Fatal("original changed")
			}
		})
	}
}

func TestPatchExistingEnvironmentReferences(t *testing.T) {
	t.Setenv("PATCH_TEST_BOOL", "true")
	t.Setenv("PATCH_TEST_INT", "5")
	t.Setenv("PATCH_TEST_FLOAT", "1.25")
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "pulse_enabled: \"${PATCH_TEST_BOOL}\"\nagent_max_iterations: \"${PATCH_TEST_INT}\"\nusage_limit_daily_usd: \"${PATCH_TEST_FLOAT}\"\nlog_level: info\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PatchYAML(path, map[string]any{"log_level": "debug"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"${PATCH_TEST_BOOL}", "${PATCH_TEST_INT}", "${PATCH_TEST_FLOAT}"} {
		if !strings.Contains(string(raw), ref) {
			t.Fatalf("lost reference %s", ref)
		}
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PulseEnabled || cfg.AgentMaxIterations != 5 || cfg.UsageLimitDailyUSD != 1.25 {
		t.Fatal("incorrect resolved values")
	}
}

func TestPatchValidRangesAndTier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	updates := map[string]any{"agent_max_iterations": 5, "usage_daily_token_budget": 0, "usage_limit_daily_usd": 0.0, "llm_providers": map[string]LLMProviderSettings{"p": {Kind: "openai"}}, "llm_tiers": map[string]LLMTierBinding{"heavy": {Provider: "p", Model: "some-model"}, "light": {Provider: "p", Model: "some-model"}, "standard": {Provider: "p", Model: "some-model", MaxTokens: 100, ThinkingBudget: 50}}}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatal(err)
	}
}
