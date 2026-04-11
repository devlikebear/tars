package tarsserver

import (
	"fmt"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

// buildLLMRouter constructs a 3-tier llm.Router from the resolved config.
// Each tier gets its own llm.Client (may be identical when the operator
// has not overridden the tier-specific fields, which matches legacy
// behavior). All clients are wrapped with usage.TrackedClient so that
// metering remains accurate per (provider, model) and tier.
//
// The provider pool schema (cfg.LLMProviders + cfg.LLMTiers) is read
// through config.ResolveAllLLMTiers, which merges pool + binding into a
// flat ResolvedLLMTier per tier.
//
// The caller is responsible for calling router.Close() on shutdown.
func buildLLMRouter(cfg config.Config, tracker *usage.Tracker) (llm.Router, error) {
	resolved, err := config.ResolveAllLLMTiers(&cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve llm tiers: %w", err)
	}

	tiers := make(map[llm.Tier]llm.TierEntry, len(resolved))
	for _, r := range resolved {
		tier, err := llm.ParseTier(r.Tier)
		if err != nil {
			return nil, fmt.Errorf("tier %q: %w", r.Tier, err)
		}
		client, err := llm.NewProvider(llm.ProviderOptions{
			Provider:        r.Kind,
			AuthMode:        r.AuthMode,
			OAuthProvider:   r.OAuthProvider,
			BaseURL:         r.BaseURL,
			WorkDir:         cfg.WorkspaceDir,
			Model:           r.Model,
			APIKey:          r.APIKey,
			ReasoningEffort: r.ReasoningEffort,
			ThinkingBudget:  r.ThinkingBudget,
			ServiceTier:     r.ServiceTier,
		})
		if err != nil {
			return nil, fmt.Errorf("tier %s: %w", r.Tier, err)
		}
		tracked := usage.NewTrackedClient(client, tracker, r.Kind, r.Model, tier)
		tiers[tier] = llm.TierEntry{
			Client:   tracked,
			Provider: r.Kind,
			Model:    r.Model,
		}
	}

	defaultTier, err := llm.ParseTier(cfg.LLMDefaultTier)
	if err != nil {
		return nil, fmt.Errorf("invalid llm_default_tier %q: %w", cfg.LLMDefaultTier, err)
	}
	if _, ok := tiers[defaultTier]; !ok {
		return nil, fmt.Errorf("llm_default_tier %q is not present in llm_tiers", defaultTier)
	}

	roleDefaults := make(map[llm.Role]llm.Tier, len(cfg.LLMRoleDefaults))
	for name, tierName := range cfg.LLMRoleDefaults {
		role, ok := llm.ParseRole(name)
		if !ok {
			continue
		}
		tier, err := llm.ParseTier(tierName)
		if err != nil {
			continue
		}
		roleDefaults[role] = tier
	}

	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers:        tiers,
		DefaultTier:  defaultTier,
		RoleDefaults: roleDefaults,
	})
	if err != nil {
		return nil, fmt.Errorf("build llm router: %w", err)
	}
	return router, nil
}
