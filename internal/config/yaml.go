package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var configYAMLRootAliases = map[string][]string{
	"automation": {},
	"extensions": {},
	"runtime":    {},
}

var configYAMLPathAliases = map[string]string{
	"api.dashboard.auth_mode":     "dashboard_auth_mode",
	"channels.telegram.bot_token": "telegram_bot_token",
	"dashboard.auth_mode":         "dashboard_auth_mode",
	"gateway.agents.catalog":      "gateway_agents_json",
	"gateway.agents.list":         "gateway_agents_json",
	"telegram.bot_token":          "telegram_bot_token",
	"usage.limits.daily_usd":      "usage_limit_daily_usd",
	"usage.limits.mode":           "usage_limit_mode",
	"usage.limits.monthly_usd":    "usage_limit_monthly_usd",
	"usage.limits.weekly_usd":     "usage_limit_weekly_usd",
	"usage.price_overrides":       "usage_price_overrides_json",
}

func loadYAML(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config file %q: %w", path, err)
	}

	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}

	flattened := flattenConfigYAML(parsed)
	var cfg Config
	for _, key := range sortedConfigYAMLKeys(flattened) {
		value := yamlValueString(flattened[key])
		if field, ok := configInputFieldByYAMLKey(key); ok {
			field.apply(&cfg, value)
		}
	}
	return cfg, nil
}

func flattenConfigYAML(parsed map[string]any) map[string]any {
	flattened := map[string]any{}
	explicit := map[string]struct{}{}
	leafTopLevel := map[string]struct{}{}

	for _, key := range sortedConfigYAMLKeys(parsed) {
		normalized := normalizeConfigYAMLPathSegment(key)
		resolved, ok := resolveConfigYAMLPath([]string{normalized}, parsed[key])
		if !ok {
			continue
		}
		flattened[resolved] = parsed[key]
		explicit[resolved] = struct{}{}
		leafTopLevel[normalized] = struct{}{}
	}

	for _, key := range sortedConfigYAMLKeys(parsed) {
		normalized := normalizeConfigYAMLPathSegment(key)
		if _, ok := leafTopLevel[normalized]; ok {
			continue
		}
		walkNestedConfigYAML(flattened, explicit, []string{normalized}, parsed[key])
	}

	return flattened
}

func walkNestedConfigYAML(dst map[string]any, explicit map[string]struct{}, path []string, raw any) {
	if resolved, ok := resolveConfigYAMLPath(path, raw); ok {
		if _, taken := explicit[resolved]; taken {
			return
		}
		if _, taken := dst[resolved]; taken {
			return
		}
		dst[resolved] = raw
		return
	}

	childMap, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for _, key := range sortedConfigYAMLKeys(childMap) {
		walkNestedConfigYAML(dst, explicit, append(path, normalizeConfigYAMLPathSegment(key)), childMap[key])
	}
}

func resolveConfigYAMLPath(path []string, raw any) (string, bool) {
	if len(path) == 0 {
		return "", false
	}

	dotPath := strings.Join(path, ".")
	if alias, ok := configYAMLPathAliases[dotPath]; ok {
		return alias, true
	}

	parts := append([]string(nil), path...)
	if alias, ok := configYAMLRootAliases[parts[0]]; ok {
		parts = append(append([]string(nil), alias...), parts[1:]...)
	}
	if len(parts) == 0 {
		return "", false
	}

	candidate := strings.Join(parts, "_")
	if _, ok := configInputFieldByYAMLKey(candidate); ok {
		return candidate, true
	}
	if isConfigYAMLSequence(raw) {
		jsonCandidate := candidate + "_json"
		if _, ok := configInputFieldByYAMLKey(jsonCandidate); ok {
			return jsonCandidate, true
		}
	}
	return "", false
}

func sortedConfigYAMLKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeConfigYAMLPathSegment(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func isConfigYAMLScalar(raw any) bool {
	switch raw.(type) {
	case nil:
		return true
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

func isConfigYAMLSequence(raw any) bool {
	switch raw.(type) {
	case []any:
		return true
	default:
		return false
	}
}

func yamlValueString(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return os.ExpandEnv(strings.TrimSpace(value))
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(value))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		return strings.TrimSpace(string(encoded))
	}
}
