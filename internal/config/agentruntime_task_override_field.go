package config

import (
	"encoding/json"
	"os"
	"strings"
)

func parseAgentRuntimeTaskOverrideJSON(raw string, fallback AgentRuntimeTaskOverrideConfig) AgentRuntimeTaskOverrideConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	var parsed AgentRuntimeTaskOverrideConfig
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fallback
	}
	return AgentRuntimeTaskOverrideConfig{
		Enabled:        parsed.Enabled,
		AllowedAliases: normalizeAgentRuntimeOverrideList(parsed.AllowedAliases),
		AllowedModels:  normalizeAgentRuntimeOverrideList(parsed.AllowedModels),
	}
}

func normalizeAgentRuntimeOverrideList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := os.ExpandEnv(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func agentRuntimeTaskOverrideField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.AgentRuntimeTaskOverride = parseAgentRuntimeTaskOverrideJSON(raw, cfg.AgentRuntimeTaskOverride)
		},
		merge: func(dst *Config, src Config) {
			if src.AgentRuntimeTaskOverride.Enabled {
				dst.AgentRuntimeTaskOverride.Enabled = true
			}
			if len(src.AgentRuntimeTaskOverride.AllowedAliases) > 0 {
				dst.AgentRuntimeTaskOverride.AllowedAliases = append([]string(nil), src.AgentRuntimeTaskOverride.AllowedAliases...)
			}
			if len(src.AgentRuntimeTaskOverride.AllowedModels) > 0 {
				dst.AgentRuntimeTaskOverride.AllowedModels = append([]string(nil), src.AgentRuntimeTaskOverride.AllowedModels...)
			}
		},
	}
}
