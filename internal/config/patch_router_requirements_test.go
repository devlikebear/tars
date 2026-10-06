package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

func TestPatchRouterRequirements(t *testing.T) {
	complete := func() map[string]LLMTierBinding {
		tiers := map[string]LLMTierBinding{}
		for _, tier := range llm.AllTiers() {
			tiers[tier.String()] = LLMTierBinding{Provider: "p", Model: "some-model"}
		}
		return tiers
	}
	t.Run("unknown_tier_preserves_valid_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := PatchYAML(path, map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: "openai"}}, "llm_tiers": complete()}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := PatchYAML(path, map[string]any{"llm_tiers": map[string]LLMTierBinding{"experimental": {Provider: "p", Model: "some-model"}}}); err == nil {
			t.Fatal("unknown tier accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatal("rejected patch changed original")
		}
	})
	for _, omitted := range llm.AllTiers() {
		t.Run("missing_"+omitted.String(), func(t *testing.T) {
			tiers := complete()
			delete(tiers, omitted.String())
			assertPatchRejectedUnchanged(t, map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: "openai"}}, "llm_tiers": tiers})
		})
	}
	t.Run("unsupported_kind", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := PatchYAML(path, map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: "openai", APIKey: "test-key"}}, "llm_tiers": complete()}); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := PatchYAML(path, map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: "not-a-provider"}}}); err == nil {
			t.Fatal("unsupported provider accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatal("rejected patch changed original")
		}
	})
	for _, kind := range []string{"openai", "kimi", "gemini", "gemini-native", "anthropic", "openai-codex", "claude-code-cli", "antigravity-cli", " OPENAI "} {
		t.Run("supported_"+kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := PatchYAML(path, map[string]any{"llm_providers": map[string]LLMProviderSettings{"p": {Kind: kind}}, "llm_tiers": complete()}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertPatchRejectedUnchanged(t *testing.T, updates map[string]any) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	const original = "log_level: info\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PatchYAML(path, updates); err == nil {
		t.Fatal("invalid router configuration accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Fatal("rejected patch changed original")
	}
}
