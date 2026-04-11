package tarsserver

import (
	"fmt"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

// buildLLMRouter constructs a 3-tier llm.Router from the resolved config.
//
// The provider pool schema (cfg.LLMProviders + cfg.LLMTiers) is read
// through config.ResolveAllLLMTiers, which merges pool + binding into a
// flat ResolvedLLMTier per tier. Each tier gets its own llm.Client and
// is wrapped with usage.TrackedClient for per-(kind, model) metering.
//
// Errors loudly when:
//   - any required tier is missing from cfg.LLMTiers
//   - a tier references an unknown provider alias
//   - cfg.LLMDefaultTier is not a key in cfg.LLMTiers
//   - llm.NewProvider rejects the resolved (Kind, Model) combination
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
		tracked := usage.NewTrackedClient(client, tracker, r.Kind, r.Model)
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
