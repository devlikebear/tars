package tarsserver

import (
	"reflect"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func tierOptionsTestConfig(binding config.LLMTierBinding) config.Config {
	return config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"pool": {Kind: "anthropic", AuthMode: "api-key", APIKey: "k", BaseURL: "https://api.anthropic.com"},
			},
			LLMTiers:       map[string]config.LLMTierBinding{"heavy": binding},
			LLMDefaultTier: "heavy",
		},
	}
}

func TestAgentRuntimeTierOptions_CarriesOutputAndBetaKnobs(t *testing.T) {
	// The console reads this DTO to show what a tier will actually send;
	// a knob missing here is a knob the operator cannot see.
	cfg := tierOptionsTestConfig(config.LLMTierBinding{
		Provider:     "pool",
		Model:        "claude-opus-5",
		MaxTokens:    32000,
		BetaFeatures: []string{"context-1m-2025-08-07"},
	})

	_, byName := agentRuntimeTierOptions(cfg)
	heavy, ok := byName["heavy"]
	if !ok {
		t.Fatal("heavy tier missing from options")
	}
	if heavy.Error != "" {
		t.Fatalf("unexpected resolve error: %s", heavy.Error)
	}
	if heavy.MaxTokens != 32000 {
		t.Errorf("MaxTokens = %d, want 32000", heavy.MaxTokens)
	}
	if !reflect.DeepEqual(heavy.BetaFeatures, []string{"context-1m-2025-08-07"}) {
		t.Errorf("BetaFeatures = %v, want [context-1m-2025-08-07]", heavy.BetaFeatures)
	}
}

func TestAgentRuntimeTierOptions_ReportsContradictoryBudgetAsTierError(t *testing.T) {
	// The resolver rejects a thinking budget that cannot fit under the
	// ceiling; the console must surface that instead of showing a tier
	// that looks fine and then fails to serve a run.
	cfg := tierOptionsTestConfig(config.LLMTierBinding{
		Provider:       "pool",
		Model:          "claude-opus-5",
		MaxTokens:      8000,
		ThinkingBudget: 8000,
	})

	_, byName := agentRuntimeTierOptions(cfg)
	heavy := byName["heavy"]
	if heavy.Error == "" {
		t.Fatal("expected a tier error for thinking_budget >= max_tokens")
	}
	if heavy.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0 on an unresolvable tier", heavy.MaxTokens)
	}
}
