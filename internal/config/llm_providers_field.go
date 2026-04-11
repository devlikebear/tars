package config

import (
	"encoding/json"
	"maps"
	"os"
	"strings"
)

// parseLLMProvidersJSON parses a JSON-encoded map alias → provider
// settings and returns a normalized copy.
//
// String values inside each entry are env-expanded via os.ExpandEnv so
// that ${ANTHROPIC_API_KEY}-style placeholders resolve correctly even
// when they appear as nested values in the YAML source (the shared YAML
// loader only expands env vars on top-level scalar values).
//
// The alias key is trimmed. Empty aliases are skipped. On parse error
// or empty result, fallback is returned unchanged.
func parseLLMProvidersJSON(raw string, fallback map[string]LLMProviderSettings) map[string]LLMProviderSettings {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	var parsed map[string]LLMProviderSettings
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fallback
	}
	out := make(map[string]LLMProviderSettings, len(parsed))
	for alias, p := range parsed {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		out[alias] = LLMProviderSettings{
			Kind:          os.ExpandEnv(strings.TrimSpace(p.Kind)),
			AuthMode:      os.ExpandEnv(strings.TrimSpace(p.AuthMode)),
			OAuthProvider: os.ExpandEnv(strings.TrimSpace(p.OAuthProvider)),
			BaseURL:       os.ExpandEnv(strings.TrimSpace(p.BaseURL)),
			APIKey:        os.ExpandEnv(strings.TrimSpace(p.APIKey)),
			ServiceTier:   os.ExpandEnv(strings.TrimSpace(p.ServiceTier)),
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// parseLLMTiersJSON parses a JSON-encoded map tier → binding and
// returns a normalized copy. Tier keys are lowercased+trimmed. String
// values inside each binding are env-expanded for consistency with
// parseLLMProvidersJSON.
func parseLLMTiersJSON(raw string, fallback map[string]LLMTierBinding) map[string]LLMTierBinding {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	var parsed map[string]LLMTierBinding
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fallback
	}
	out := make(map[string]LLMTierBinding, len(parsed))
	for tier, b := range parsed {
		tier = strings.ToLower(strings.TrimSpace(tier))
		if tier == "" {
			continue
		}
		out[tier] = LLMTierBinding{
			Provider:        os.ExpandEnv(strings.TrimSpace(b.Provider)),
			Model:           os.ExpandEnv(strings.TrimSpace(b.Model)),
			ReasoningEffort: os.ExpandEnv(strings.TrimSpace(b.ReasoningEffort)),
			ThinkingBudget:  b.ThinkingBudget,
			ServiceTier:     os.ExpandEnv(strings.TrimSpace(b.ServiceTier)),
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// parseLLMRoleDefaultsJSON parses a JSON-encoded map role → tier and
// returns a normalized copy. Both keys and values are lowercased +
// trimmed; empty entries are dropped.
//
// Unlike the legacy normalizeRoleDefaults path, this parser does NOT
// reject unknown role names — validation happens at router build time
// via llm.ParseRole, so the config parser stays free of an internal/llm
// import.
func parseLLMRoleDefaultsJSON(raw string, fallback map[string]string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fallback
	}
	out := make(map[string]string, len(parsed))
	for role, tier := range parsed {
		role = strings.ToLower(strings.TrimSpace(role))
		tier = strings.ToLower(strings.TrimSpace(tier))
		if role == "" || tier == "" {
			continue
		}
		out[role] = tier
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// llmProvidersField builds the configInputField entry for llm_providers.
func llmProvidersField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.LLMProviders = parseLLMProvidersJSON(raw, cfg.LLMProviders)
		},
		merge: func(dst *Config, src Config) {
			if len(src.LLMProviders) == 0 {
				return
			}
			dst.LLMProviders = cloneLLMProviders(src.LLMProviders)
		},
	}
}

// llmTiersField builds the configInputField entry for llm_tiers.
func llmTiersField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.LLMTiers = parseLLMTiersJSON(raw, cfg.LLMTiers)
		},
		merge: func(dst *Config, src Config) {
			if len(src.LLMTiers) == 0 {
				return
			}
			dst.LLMTiers = cloneLLMTiers(src.LLMTiers)
		},
	}
}

// llmRoleDefaultsField builds the configInputField entry for
// llm_role_defaults (nested map form, replacing the legacy flat
// llm_role_* fields in the cutover commit).
func llmRoleDefaultsField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.LLMRoleDefaults = parseLLMRoleDefaultsJSON(raw, cfg.LLMRoleDefaults)
		},
		merge: func(dst *Config, src Config) {
			if len(src.LLMRoleDefaults) == 0 {
				return
			}
			dst.LLMRoleDefaults = cloneStringMap(src.LLMRoleDefaults)
		},
	}
}

func cloneLLMProviders(src map[string]LLMProviderSettings) map[string]LLMProviderSettings {
	cloned := make(map[string]LLMProviderSettings, len(src))
	maps.Copy(cloned, src)
	return cloned
}

func cloneLLMTiers(src map[string]LLMTierBinding) map[string]LLMTierBinding {
	cloned := make(map[string]LLMTierBinding, len(src))
	maps.Copy(cloned, src)
	return cloned
}

func cloneStringMap(src map[string]string) map[string]string {
	cloned := make(map[string]string, len(src))
	maps.Copy(cloned, src)
	return cloned
}
