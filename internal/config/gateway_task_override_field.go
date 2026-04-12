package config

import (
	"encoding/json"
	"os"
	"strings"
)

func parseGatewayTaskOverrideJSON(raw string, fallback GatewayTaskOverrideConfig) GatewayTaskOverrideConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	var parsed GatewayTaskOverrideConfig
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return fallback
	}
	return GatewayTaskOverrideConfig{
		Enabled:        parsed.Enabled,
		AllowedAliases: normalizeGatewayOverrideList(parsed.AllowedAliases),
		AllowedModels:  normalizeGatewayOverrideList(parsed.AllowedModels),
	}
}

func normalizeGatewayOverrideList(values []string) []string {
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

func gatewayTaskOverrideField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.GatewayTaskOverride = parseGatewayTaskOverrideJSON(raw, cfg.GatewayTaskOverride)
		},
		merge: func(dst *Config, src Config) {
			if src.GatewayTaskOverride.Enabled {
				dst.GatewayTaskOverride.Enabled = true
			}
			if len(src.GatewayTaskOverride.AllowedAliases) > 0 {
				dst.GatewayTaskOverride.AllowedAliases = append([]string(nil), src.GatewayTaskOverride.AllowedAliases...)
			}
			if len(src.GatewayTaskOverride.AllowedModels) > 0 {
				dst.GatewayTaskOverride.AllowedModels = append([]string(nil), src.GatewayTaskOverride.AllowedModels...)
			}
		},
	}
}
