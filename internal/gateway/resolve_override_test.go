package gateway

import (
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestResolveOverride(t *testing.T) {
	cfg := &config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"anthropic_prod": {Kind: "anthropic", AuthMode: "api-key", APIKey: "prod-key", BaseURL: "https://prod.example"},
				"anthropic_dev":  {Kind: "anthropic", AuthMode: "api-key", APIKey: "dev-key", BaseURL: "https://dev.example"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"heavy":    {Provider: "anthropic_prod", Model: "claude-opus"},
				"standard": {Provider: "anthropic_prod", Model: "claude-sonnet"},
				"light":    {Provider: "anthropic_prod", Model: "claude-haiku"},
			},
			LLMDefaultTier:  "standard",
			LLMRoleDefaults: map[string]string{"gateway_default": "light"},
		},
		GatewayConfig: config.GatewayConfig{
			GatewayTaskOverride: config.GatewayTaskOverrideConfig{
				Enabled:        true,
				AllowedAliases: []string{"anthropic_prod", "anthropic_dev"},
			},
		},
	}

	tests := []struct {
		name           string
		tier           string
		override       *ProviderOverride
		overrideSource string
		wantAlias      string
		wantModel      string
		wantSource     string
	}{
		{name: "tier fallback", tier: "heavy", wantAlias: "anthropic_prod", wantModel: "claude-opus", wantSource: "tier"},
		{name: "role default fallback", wantAlias: "anthropic_prod", wantModel: "claude-haiku", wantSource: "tier"},
		{name: "task override alias only", tier: "heavy", override: &ProviderOverride{Alias: "anthropic_dev"}, overrideSource: "task", wantAlias: "anthropic_dev", wantModel: "claude-opus", wantSource: "task"},
		{name: "task override alias and model", tier: "light", override: &ProviderOverride{Alias: "anthropic_dev", Model: "claude-opus-override"}, overrideSource: "task", wantAlias: "anthropic_dev", wantModel: "claude-opus-override", wantSource: "task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, meta, err := ResolveOverride(cfg, tt.tier, tt.override, tt.overrideSource)
			if err != nil {
				t.Fatalf("ResolveOverride: %v", err)
			}
			if resolved.ProviderAlias != tt.wantAlias || resolved.Model != tt.wantModel {
				t.Fatalf("unexpected resolved tier: %+v", resolved)
			}
			if meta.ResolvedAlias != tt.wantAlias || meta.ResolvedModel != tt.wantModel || meta.OverrideSource != tt.wantSource {
				t.Fatalf("unexpected metadata: %+v", meta)
			}
		})
	}
}

func TestResolveOverride_RejectsDisabledAndAllowlistViolations(t *testing.T) {
	cfg := &config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"anthropic_prod": {Kind: "anthropic", AuthMode: "api-key", APIKey: "prod-key", BaseURL: "https://prod.example"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"standard": {Provider: "anthropic_prod", Model: "claude-sonnet"},
			},
			LLMDefaultTier: "standard",
		},
	}
	if _, _, err := ResolveOverride(cfg, "standard", &ProviderOverride{Alias: "anthropic_prod"}, "task"); err == nil {
		t.Fatalf("expected disabled override to fail")
	}
	cfg.GatewayTaskOverride.Enabled = true
	cfg.GatewayTaskOverride.AllowedAliases = []string{"anthropic_other"}
	if _, _, err := ResolveOverride(cfg, "standard", &ProviderOverride{Alias: "anthropic_prod"}, "task"); err == nil {
		t.Fatalf("expected allowlist violation to fail")
	}
}
