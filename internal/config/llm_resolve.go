package config

import (
	"fmt"
	"strings"
)

// ResolvedLLMTier is the flat, final view of one tier's effective LLM
// configuration after merging the named provider pool entry and the
// tier binding. The router builder consumes this struct directly —
// callers never read cfg.LLMProviders or cfg.LLMTiers in isolation.
//
// ProviderAlias records which pool entry served this tier so that
// tars doctor and startup logs can show provenance (e.g.
// "heavy → codex / gpt-5.4").
type ResolvedLLMTier struct {
	Tier string

	// From the referenced provider pool entry
	Kind          string
	AuthMode      string
	OAuthProvider string
	BaseURL       string
	APIKey        string

	// From the tier binding
	Model           string
	ReasoningEffort string
	ThinkingBudget  int

	// ServiceTier: binding override when set, otherwise provider default
	ServiceTier string

	// Provenance — alias of the provider pool entry that served this tier
	ProviderAlias string
}

// ResolveLLMTier returns the effective settings for the given tier.
// The tier name is normalized (lowercased, trimmed) before lookup.
//
// Errors (all loud — no silent fallback):
//
//   - cfg is nil
//   - tier is empty
//   - cfg.LLMTiers[tier] is missing
//   - binding.Provider is empty
//   - cfg.LLMProviders[binding.Provider] is missing
//   - resolved Kind is empty
//   - binding.Model is empty
//
// Kind is normalized to lowercase; other string fields are trimmed.
// Kind value is NOT validated against a closed list — llm.NewProvider
// rejects unknown kinds with a clear error at router build time, and
// the config package must stay free of an internal/llm import.
func ResolveLLMTier(cfg *Config, tier string) (ResolvedLLMTier, error) {
	if cfg == nil {
		return ResolvedLLMTier{}, fmt.Errorf("resolve llm tier: nil config")
	}
	tierNorm := strings.ToLower(strings.TrimSpace(tier))
	if tierNorm == "" {
		return ResolvedLLMTier{}, fmt.Errorf("resolve llm tier: empty tier name")
	}

	binding, ok := cfg.LLMTiers[tierNorm]
	if !ok {
		return ResolvedLLMTier{}, fmt.Errorf("tier %q not configured in llm_tiers", tierNorm)
	}

	alias := strings.TrimSpace(binding.Provider)
	if alias == "" {
		return ResolvedLLMTier{}, fmt.Errorf("tier %q binding has empty provider alias", tierNorm)
	}

	provider, ok := cfg.LLMProviders[alias]
	if !ok {
		return ResolvedLLMTier{}, fmt.Errorf("tier %q references unknown provider alias %q", tierNorm, alias)
	}

	kind := strings.ToLower(strings.TrimSpace(provider.Kind))
	if kind == "" {
		return ResolvedLLMTier{}, fmt.Errorf("provider %q has empty kind", alias)
	}

	model := strings.TrimSpace(binding.Model)
	if model == "" {
		return ResolvedLLMTier{}, fmt.Errorf("tier %q binding has empty model", tierNorm)
	}

	// ServiceTier: binding override wins over provider default
	serviceTier := strings.TrimSpace(binding.ServiceTier)
	if serviceTier == "" {
		serviceTier = strings.TrimSpace(provider.ServiceTier)
	}

	return ResolvedLLMTier{
		Tier:            tierNorm,
		Kind:            kind,
		AuthMode:        strings.ToLower(strings.TrimSpace(provider.AuthMode)),
		OAuthProvider:   strings.ToLower(strings.TrimSpace(provider.OAuthProvider)),
		BaseURL:         strings.TrimSpace(provider.BaseURL),
		APIKey:          strings.TrimSpace(provider.APIKey),
		Model:           model,
		ReasoningEffort: strings.TrimSpace(binding.ReasoningEffort),
		ThinkingBudget:  binding.ThinkingBudget,
		ServiceTier:     serviceTier,
		ProviderAlias:   alias,
	}, nil
}

// ResolveAllLLMTiers resolves every tier present in cfg.LLMTiers and
// returns them keyed by normalized tier name. It is the single entry
// point used by buildLLMRouter — callers should not iterate
// cfg.LLMTiers directly.
//
// Returns the first resolution error encountered (fail loud).
//
// Note: this function does NOT enforce that heavy/standard/light are
// all present. That check lives in the router construction path
// (llm.NewRouter) so that the error message can reference the tier
// interface rather than the config schema.
func ResolveAllLLMTiers(cfg *Config) (map[string]ResolvedLLMTier, error) {
	if cfg == nil {
		return nil, fmt.Errorf("resolve all llm tiers: nil config")
	}
	out := make(map[string]ResolvedLLMTier, len(cfg.LLMTiers))
	for tier := range cfg.LLMTiers {
		resolved, err := ResolveLLMTier(cfg, tier)
		if err != nil {
			return nil, err
		}
		out[resolved.Tier] = resolved
	}
	return out, nil
}
