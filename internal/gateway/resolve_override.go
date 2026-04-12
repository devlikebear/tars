package gateway

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
)

func ResolveOverride(cfg *config.Config, tier string, override *ProviderOverride, overrideSource string) (config.ResolvedLLMTier, PromptExecutionMetadata, error) {
	if cfg == nil {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("override failed: nil config")
	}
	baseTier := strings.ToLower(strings.TrimSpace(tier))
	if baseTier == "" {
		baseTier = strings.ToLower(strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleGatewayDefault)]))
		if baseTier == "" {
			baseTier = strings.ToLower(strings.TrimSpace(cfg.LLMDefaultTier))
		}
	}
	resolved, err := config.ResolveLLMTier(cfg, baseTier)
	if err != nil {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, err
	}
	metadata := PromptExecutionMetadata{
		ResolvedAlias:  resolved.ProviderAlias,
		ResolvedKind:   resolved.Kind,
		ResolvedModel:  resolved.Model,
		OverrideSource: "tier",
	}
	effectiveOverride := CloneProviderOverride(override)
	if effectiveOverride == nil {
		return resolved, metadata, nil
	}
	if !cfg.GatewayTaskOverride.Enabled {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("task override rejected: gateway_task_override is disabled")
	}
	alias := strings.TrimSpace(effectiveOverride.Alias)
	if alias == "" {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("task override rejected: alias is required")
	}
	if len(cfg.GatewayTaskOverride.AllowedAliases) > 0 && !containsExactString(cfg.GatewayTaskOverride.AllowedAliases, alias) {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("task override rejected: alias %q not in gateway_task_override.allowed_aliases", alias)
	}
	provider, ok := cfg.LLMProviders[alias]
	if !ok {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("task override rejected: alias %q not registered in llm_providers", alias)
	}
	model := strings.TrimSpace(effectiveOverride.Model)
	if model == "" {
		model = resolved.Model
	}
	if len(cfg.GatewayTaskOverride.AllowedModels) > 0 && !containsExactString(cfg.GatewayTaskOverride.AllowedModels, model) {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("task override rejected: model %q not in gateway_task_override.allowed_models", model)
	}
	resolved.ProviderAlias = alias
	resolved.Kind = strings.ToLower(strings.TrimSpace(provider.Kind))
	resolved.AuthMode = strings.ToLower(strings.TrimSpace(provider.AuthMode))
	resolved.OAuthProvider = strings.ToLower(strings.TrimSpace(provider.OAuthProvider))
	resolved.BaseURL = strings.TrimSpace(provider.BaseURL)
	resolved.APIKey = strings.TrimSpace(provider.APIKey)
	resolved.Model = model
	if serviceTier := strings.TrimSpace(provider.ServiceTier); serviceTier != "" {
		resolved.ServiceTier = serviceTier
	}
	if resolved.APIKey == "" && resolved.AuthMode == "api-key" {
		return config.ResolvedLLMTier{}, PromptExecutionMetadata{}, fmt.Errorf("override failed: provider alias %q missing api_key", alias)
	}
	metadata.ResolvedAlias = resolved.ProviderAlias
	metadata.ResolvedKind = resolved.Kind
	metadata.ResolvedModel = resolved.Model
	metadata.OverrideSource = strings.TrimSpace(overrideSource)
	if metadata.OverrideSource == "" {
		metadata.OverrideSource = "task"
	}
	return resolved, metadata, nil
}

func containsExactString(values []string, target string) bool {
	needle := strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}
