package config

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llmdefaults"
)

// ResolvedLLMTier is the flat, final view of one tier's effective LLM
// configuration after merging the named provider pool entry and the
// tier binding. The router builder consumes this struct directly —
// callers never read cfg.LLMProviders or cfg.LLMTiers in isolation.
//
// ProviderAlias records which pool entry served this tier so that
// tars doctor and startup logs can show provenance (e.g.
// "heavy → codex / gpt-5.4").
//
// OAuthProvider is derived from Kind via llmdefaults.OAuthProvider when
// AuthMode is "oauth"; it is not user-configurable. ServiceTier comes
// from the tier binding only — there is no provider-level fallback.
type ResolvedLLMTier struct {
	Tier string

	// From the referenced provider pool entry
	Kind          string
	AuthMode      string
	OAuthProvider string // derived from Kind, see llmdefaults.OAuthProvider
	BaseURL       string
	APIKey        string

	// From the tier binding
	Model           string
	ReasoningEffort string
	ThinkingBudget  int
	ServiceTier     string

	// MaxTokens is the tier's raw output ceiling; 0 means the binding did
	// not set one. Per-model defaulting deliberately does NOT happen here —
	// it lives in the provider layer so that external consumers calling
	// llm.NewProvider directly get the same defaults as the TARS router.
	MaxTokens int

	// BetaFeatures are provider beta flags to opt this tier into, already
	// trimmed and de-duplicated. Nil means send none.
	BetaFeatures []string

	// ContextWindow is the tier's effective input+output window, already
	// defaulted from the model where the binding left it unset. 0 means
	// unknown — a gateway-hosted model with no documented window — and
	// history budgeting stays on the global compaction settings.
	//
	// Unlike MaxTokens, this IS resolved here rather than in the provider
	// layer: nothing in internal/llm consumes it, and the consumer that
	// does (history budgeting) reads the resolved tier.
	ContextWindow int

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
//   - binding sets both max_tokens and thinking_budget, and the budget is
//     not strictly smaller (the budget is spent out of the output ceiling)
//   - binding sets thinking_budget on a kind: anthropic model that sets
//     reasoning depth by effort and rejects a token budget
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

	// Guard only when the binding states both values. An unset max_tokens
	// resolves to a per-model default the config layer cannot see, and
	// hard-erroring against a guess would refuse to boot configs that work
	// today — the provider degrades thinking off with a warning in that case.
	if binding.MaxTokens > 0 && binding.ThinkingBudget >= binding.MaxTokens {
		return ResolvedLLMTier{}, fmt.Errorf(
			"tier %q: thinking_budget (%d) must be less than max_tokens (%d) — the thinking budget is spent out of the output ceiling",
			tierNorm, binding.ThinkingBudget, binding.MaxTokens)
	}

	// A token budget cannot be expressed on a model that sets reasoning
	// depth by effort — the provider rejects the parameter outright. This
	// only fires on a pairing that already fails on every request, so it
	// turns a per-turn 400 into one startup error naming the fix.
	if binding.ThinkingBudget > 0 && kind == llmdefaults.ProviderAnthropic {
		if behavior, _ := llmdefaults.ModelBehaviorFor(model); behavior.Thinking == llmdefaults.ThinkingModeAdaptive {
			return ResolvedLLMTier{}, fmt.Errorf(
				"tier %q: model %q does not accept thinking_budget (%d) — it sets reasoning depth by effort; use reasoning_effort instead",
				tierNorm, model, binding.ThinkingBudget)
		}
	}

	authMode := strings.ToLower(strings.TrimSpace(provider.AuthMode))
	oauthProvider := ""
	if authMode == "oauth" {
		oauthProvider = strings.ToLower(strings.TrimSpace(llmdefaults.OAuthProvider(kind)))
	}

	return ResolvedLLMTier{
		Tier:            tierNorm,
		Kind:            kind,
		AuthMode:        authMode,
		OAuthProvider:   oauthProvider,
		BaseURL:         strings.TrimSpace(provider.BaseURL),
		APIKey:          strings.TrimSpace(provider.APIKey),
		Model:           model,
		ReasoningEffort: strings.TrimSpace(binding.ReasoningEffort),
		ThinkingBudget:  binding.ThinkingBudget,
		ServiceTier:     strings.TrimSpace(binding.ServiceTier),
		MaxTokens:       binding.MaxTokens,
		BetaFeatures:    normalizeLLMBetaFeatures(binding.BetaFeatures),
		ContextWindow:   resolveContextWindow(binding.ContextWindow, model),
		ProviderAlias:   alias,
	}, nil
}

// resolveContextWindow picks the tier's effective window: an explicit
// setting, else the model's documented window, else 0 for a model TARS does
// not recognize.
//
// An explicit value wins even when it is larger than the documented window —
// it is the only way to describe a gateway or a model newer than this build,
// and the pre-flight check reports an overrun rather than the config layer
// second-guessing the operator.
func resolveContextWindow(configured int, model string) int {
	if configured > 0 {
		return configured
	}
	return llmdefaults.ContextWindow(model)
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
