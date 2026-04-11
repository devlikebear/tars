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
// metering remains accurate per (provider, model).
//
// The caller is responsible for calling router.Close() on shutdown.
func buildLLMRouter(cfg config.Config, tracker *usage.Tracker) (llm.Router, error) {
	tiers := map[llm.Tier]llm.TierEntry{}
	for _, spec := range []struct {
		tier     llm.Tier
		settings config.LLMTierSettings
	}{
		{llm.TierHeavy, cfg.LLMTierHeavy},
		{llm.TierStandard, cfg.LLMTierStandard},
		{llm.TierLight, cfg.LLMTierLight},
	} {
		client, err := llm.NewProvider(llm.ProviderOptions{
			Provider:        spec.settings.Provider,
			AuthMode:        spec.settings.AuthMode,
			OAuthProvider:   spec.settings.OAuthProvider,
			BaseURL:         spec.settings.BaseURL,
			WorkDir:         cfg.WorkspaceDir,
			Model:           spec.settings.Model,
			APIKey:          spec.settings.APIKey,
			ReasoningEffort: spec.settings.ReasoningEffort,
			ThinkingBudget:  spec.settings.ThinkingBudget,
			ServiceTier:     spec.settings.ServiceTier,
		})
		if err != nil {
			return nil, fmt.Errorf("tier %s: %w", spec.tier, err)
		}
		tracked := usage.NewTrackedClient(client, tracker, spec.settings.Provider, spec.settings.Model, spec.tier)
		tiers[spec.tier] = llm.TierEntry{
			Client:   tracked,
			Provider: spec.settings.Provider,
			Model:    spec.settings.Model,
		}
	}

	defaultTier := llm.TierStandard
	if parsed, err := llm.ParseTier(cfg.LLMDefaultTier); err == nil {
		defaultTier = parsed
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
