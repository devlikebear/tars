package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LegacyKeyWarning describes a removed config key and its migration path.
type LegacyKeyWarning struct {
	Key       string
	Migration string
}

// legacyKeyMigrations maps removed top-level YAML keys to migration guidance.
var legacyKeyMigrations = map[string]string{
	// Flat LLM fields replaced by llm_providers + llm_tiers
	"llm_provider":         "use llm_providers with a named provider entry",
	"llm_auth_mode":        "set auth_mode inside the provider entry under llm_providers",
	"llm_oauth_provider":   "set oauth_provider inside the provider entry under llm_providers",
	"llm_base_url":         "set base_url inside the provider entry under llm_providers",
	"llm_api_key":          "set api_key inside the provider entry under llm_providers",
	"llm_model":            "set model inside a tier binding under llm_tiers",
	"llm_reasoning_effort": "set reasoning_effort inside a tier binding under llm_tiers",
	"llm_thinking_budget":  "set thinking_budget inside a tier binding under llm_tiers",
	"llm_service_tier":     "set service_tier inside a tier binding under llm_tiers",

	// Heartbeat replaced by pulse
	"heartbeat_enabled":  "use pulse_enabled instead",
	"heartbeat_interval": "use pulse_interval instead",
	"heartbeat_cron":     "use pulse_interval instead",
}

// legacyKeyPrefixes identifies families of removed flat keys.
var legacyKeyPrefixes = []struct {
	prefix    string
	migration string
}{
	{"llm_tier_heavy_", "use llm_tiers.heavy with provider + model fields"},
	{"llm_tier_standard_", "use llm_tiers.standard with provider + model fields"},
	{"llm_tier_light_", "use llm_tiers.light with provider + model fields"},
	{"llm_role_", "use llm_role_defaults with role → tier mappings"},
}

// DetectLegacyKeys reads a YAML config file and returns warnings for any
// removed keys found. Returns nil if the file does not exist or contains
// no legacy keys.
func DetectLegacyKeys(path string) []LegacyKeyWarning {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil
	}

	var warnings []LegacyKeyWarning
	for key := range parsed {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if migration, ok := legacyKeyMigrations[normalized]; ok {
			warnings = append(warnings, LegacyKeyWarning{Key: key, Migration: migration})
			continue
		}
		for _, prefix := range legacyKeyPrefixes {
			if strings.HasPrefix(normalized, prefix.prefix) && normalized != "llm_role_defaults" {
				warnings = append(warnings, LegacyKeyWarning{Key: key, Migration: prefix.migration})
				break
			}
		}
	}
	return warnings
}

// FormatLegacyKeyWarnings returns a human-readable summary of legacy key
// warnings suitable for CLI output.
func FormatLegacyKeyWarnings(warnings []LegacyKeyWarning) string {
	if len(warnings) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "found %d removed config key(s):\n", len(warnings))
	for _, w := range warnings {
		fmt.Fprintf(&b, "  - %s → %s\n", w.Key, w.Migration)
	}
	return b.String()
}
