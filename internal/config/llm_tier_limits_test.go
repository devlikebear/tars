package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func limitsTestConfig(binding LLMTierBinding) Config {
	return Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{
				"pool": {Kind: "anthropic", AuthMode: "api-key", APIKey: "k", BaseURL: "https://api.anthropic.com"},
			},
			LLMTiers: map[string]LLMTierBinding{"heavy": binding},
		},
	}
}

func TestResolveLLMTier_MaxTokensPresentAbsentAndNegative(t *testing.T) {
	tests := []struct {
		name string
		set  int
		want int
	}{
		{"explicit value is carried through", 32000, 32000},
		{"absent stays 0 so the provider defaults per model", 0, 0},
		// Negative values are clamped by applyLLMPoolDefaults, but the
		// resolver must not invent a value of its own if one slips past.
		{"negative is carried through unchanged, not defaulted", -5, -5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: "claude-opus-5", MaxTokens: tc.set})
			resolved, err := ResolveLLMTier(&cfg, "heavy")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if resolved.MaxTokens != tc.want {
				t.Fatalf("MaxTokens = %d, want %d", resolved.MaxTokens, tc.want)
			}
		})
	}
}

func TestResolveLLMTier_ThinkingBudgetMustFitUnderMaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		budget    int
		wantErr   bool
	}{
		{"budget below the ceiling resolves", 32000, 8000, false},
		{"budget equal to the ceiling is rejected", 8000, 8000, true},
		{"budget above the ceiling is rejected", 8000, 16000, true},
		// max_tokens unset means the provider picks a per-model default the
		// config layer cannot see. Erroring against a guess would refuse to
		// boot configs that work today.
		{"unset ceiling does not error", 0, 100000, false},
		{"unset budget does not error", 8000, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := limitsTestConfig(LLMTierBinding{
				Provider:       "pool",
				Model:          "claude-opus-5",
				MaxTokens:      tc.maxTokens,
				ThinkingBudget: tc.budget,
			})
			_, err := ResolveLLMTier(&cfg, "heavy")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got none")
				}
				// The message must name both values so the operator can see
				// which one to change.
				for _, want := range []string{"thinking_budget", "max_tokens", "heavy"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
		})
	}
}

func TestResolveLLMTier_BetaFeaturesNormalized(t *testing.T) {
	cfg := limitsTestConfig(LLMTierBinding{
		Provider:     "pool",
		Model:        "claude-opus-5",
		BetaFeatures: []string{" context-1m-2025-08-07 ", "", "context-1m-2025-08-07", "interleaved-thinking-2025-05-14"},
	})
	resolved, err := ResolveLLMTier(&cfg, "heavy")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"}
	if !reflect.DeepEqual(resolved.BetaFeatures, want) {
		t.Fatalf("BetaFeatures = %v, want %v", resolved.BetaFeatures, want)
	}
}

func TestResolveLLMTier_BetaFeaturesEmptyResolvesToNil(t *testing.T) {
	for _, features := range [][]string{nil, {}, {"  ", ""}} {
		cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: "claude-opus-5", BetaFeatures: features})
		resolved, err := ResolveLLMTier(&cfg, "heavy")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resolved.BetaFeatures != nil {
			t.Fatalf("BetaFeatures = %v, want nil for input %v", resolved.BetaFeatures, features)
		}
	}
}

func TestResolveLLMTier_BetaFeaturesDoNotAliasTheBinding(t *testing.T) {
	features := []string{"context-1m-2025-08-07"}
	cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: "claude-opus-5", BetaFeatures: features})
	resolved, err := ResolveLLMTier(&cfg, "heavy")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	features[0] = "mutated"
	if resolved.BetaFeatures[0] != "context-1m-2025-08-07" {
		t.Fatalf("resolved tier aliased the binding's slice: %q", resolved.BetaFeatures[0])
	}
}

func TestApplyLLMPoolDefaults_ClampsAndNormalizesNewTierFields(t *testing.T) {
	cfg := limitsTestConfig(LLMTierBinding{
		Provider:     "pool",
		Model:        "claude-opus-5",
		MaxTokens:    -1,
		BetaFeatures: []string{" a ", "a", ""},
	})
	applyLLMPoolDefaults(&cfg)
	binding := cfg.LLMTiers["heavy"]
	if binding.MaxTokens != 0 {
		t.Fatalf("MaxTokens = %d, want 0 after clamping", binding.MaxTokens)
	}
	if !reflect.DeepEqual(binding.BetaFeatures, []string{"a"}) {
		t.Fatalf("BetaFeatures = %v, want [a]", binding.BetaFeatures)
	}
}

func TestCloneLLMTiers_DoesNotShareBetaFeatureBackingArray(t *testing.T) {
	// A plain maps.Copy would leave both maps pointing at one slice, so a
	// later normalization pass would rewrite bindings the caller owns.
	src := map[string]LLMTierBinding{
		"heavy": {Provider: "pool", Model: "claude-opus-5", BetaFeatures: []string{"context-1m-2025-08-07"}},
	}
	cloned := cloneLLMTiers(src)
	cloned["heavy"].BetaFeatures[0] = "mutated"
	if src["heavy"].BetaFeatures[0] != "context-1m-2025-08-07" {
		t.Fatalf("clone shares the source backing array: %q", src["heavy"].BetaFeatures[0])
	}
}

func TestParseLLMTiersJSON_CarriesNewFields(t *testing.T) {
	raw := `{"heavy":{"provider":"pool","model":"claude-opus-5","max_tokens":32000,"beta_features":["context-1m-2025-08-07","context-1m-2025-08-07"]}}`
	out := parseLLMTiersJSON(raw, nil)
	binding, ok := out["heavy"]
	if !ok {
		t.Fatal("heavy tier missing from parse result")
	}
	if binding.MaxTokens != 32000 {
		t.Fatalf("MaxTokens = %d, want 32000", binding.MaxTokens)
	}
	if !reflect.DeepEqual(binding.BetaFeatures, []string{"context-1m-2025-08-07"}) {
		t.Fatalf("BetaFeatures = %v, want one de-duplicated entry", binding.BetaFeatures)
	}
}

func TestNormalizeLLMBetaFeatures_PreservesOrder(t *testing.T) {
	got := normalizeLLMBetaFeatures([]string{"c", "a", "b", "a"})
	want := []string{"c", "a", "b"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeLLMBetaFeatures = %v, want %v", got, want)
	}
}
