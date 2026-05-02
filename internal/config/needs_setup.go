package config

import "strings"

// requiredSetupTiers lists tier names that must be fully bound for the
// LLM router to start. Mirrors the requirement enforced by
// llm.NewRouter at boot time.
var requiredSetupTiers = []string{"heavy", "standard", "light"}

// NeedsSetup returns true when the loaded config cannot start the LLM
// router. It is the single source of truth shared between the healthz
// handler, the setup status handler, and the boot path (see Phase 2).
//
// Conditions (any one triggers needs_setup=true):
//   - cfg.LLMProviders is empty
//   - cfg.LLMTiers is missing any of: heavy, standard, light
//   - any of those three tier bindings has empty (or whitespace-only)
//     provider or model
//
// This function does NOT validate that referenced provider aliases
// exist in cfg.LLMProviders — that check belongs to ResolveAllLLMTiers
// at boot time. Only structural completeness is checked here.
func NeedsSetup(cfg Config) bool {
	if len(cfg.LLMProviders) == 0 {
		return true
	}
	for _, tier := range requiredSetupTiers {
		binding, ok := cfg.LLMTiers[tier]
		if !ok {
			return true
		}
		if strings.TrimSpace(binding.Provider) == "" {
			return true
		}
		if strings.TrimSpace(binding.Model) == "" {
			return true
		}
	}
	return false
}
