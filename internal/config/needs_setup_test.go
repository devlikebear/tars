package config

import "testing"

func TestNeedsSetup(t *testing.T) {
	completeProviders := map[string]LLMProviderSettings{
		"openai": {Kind: "openai", AuthMode: "api-key", APIKey: "sk-test"},
	}
	completeTiers := map[string]LLMTierBinding{
		"heavy":    {Provider: "openai", Model: "gpt-5.4"},
		"standard": {Provider: "openai", Model: "gpt-5.4"},
		"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
	}

	makeCfg := func(providers map[string]LLMProviderSettings, tiers map[string]LLMTierBinding) Config {
		return Config{LLMConfig: LLMConfig{LLMProviders: providers, LLMTiers: tiers}}
	}

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "nil providers map",
			cfg:  makeCfg(nil, completeTiers),
			want: true,
		},
		{
			name: "empty providers map",
			cfg:  makeCfg(map[string]LLMProviderSettings{}, completeTiers),
			want: true,
		},
		{
			name: "nil tiers map",
			cfg:  makeCfg(completeProviders, nil),
			want: true,
		},
		{
			name: "missing heavy tier",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "missing standard tier",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy": {Provider: "openai", Model: "gpt-5.4"},
				"light": {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "missing light tier",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "openai", Model: "gpt-5.4"},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
			}),
			want: true,
		},
		{
			name: "tier with empty provider",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "", Model: "gpt-5.4"},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "tier with whitespace-only provider",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "   ", Model: "gpt-5.4"},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "tier with empty model",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "openai", Model: ""},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "tier with whitespace-only model",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "openai", Model: " "},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
			}),
			want: true,
		},
		{
			name: "complete config",
			cfg:  makeCfg(completeProviders, completeTiers),
			want: false,
		},
		{
			name: "complete config with extra tier (alpha)",
			cfg: makeCfg(completeProviders, map[string]LLMTierBinding{
				"heavy":    {Provider: "openai", Model: "gpt-5.4"},
				"standard": {Provider: "openai", Model: "gpt-5.4"},
				"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
				"alpha":    {Provider: "openai", Model: "gpt-experimental"},
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsSetup(tt.cfg)
			if got != tt.want {
				t.Fatalf("NeedsSetup() = %v, want %v", got, tt.want)
			}
		})
	}
}
