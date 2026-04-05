package config

import (
	"strings"
)

// Canonical tier names. Duplicated from internal/llm to avoid importing the
// llm package from config (which must stay dependency-free for use in CLI
// bootstrap). Keep these strings in sync with internal/llm/tier.go.
const (
	TierHeavy    = "heavy"
	TierStandard = "standard"
	TierLight    = "light"
)

// Canonical role names. Duplicated from internal/llm/role.go for the same
// reason as the tier constants. Keep in sync.
const (
	RoleChatMain         = "chat_main"
	RoleContextCompactor = "context_compactor"
	RoleMemoryHook       = "memory_hook"
	RoleReflectionMemory = "reflection_memory"
	RoleReflectionKB     = "reflection_kb"
	RolePulseDecider     = "pulse_decider"
	RoleGatewayDefault   = "gateway_default"
	RoleGatewayPlanner   = "gateway_planner"
)

// allKnownRoles lists every role supported by the router.
func allKnownRoles() []string {
	return []string{
		RoleChatMain,
		RoleContextCompactor,
		RoleMemoryHook,
		RoleReflectionMemory,
		RoleReflectionKB,
		RolePulseDecider,
		RoleGatewayDefault,
		RoleGatewayPlanner,
	}
}

// normalizeTierName lowercases and trims a tier string. Empty input
// yields an empty string; unknown values are returned as-is so that the
// caller can decide whether to reject or fall back.
func normalizeTierName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// isKnownTier reports whether name is one of the canonical tiers.
func isKnownTier(name string) bool {
	switch normalizeTierName(name) {
	case TierHeavy, TierStandard, TierLight:
		return true
	default:
		return false
	}
}

// EnsureLLMTierDefaults backfills tier-level LLM settings from the legacy
// flat LLM* fields when the new tier fields have been left empty. After
// this function runs, each tier is guaranteed to have a resolvable
// Provider+Model pair, even if the operator has only configured the legacy
// fields. This is the backward-compat shim that keeps existing configs
// working unchanged.
//
// Rules:
//  1. LLMDefaultTier defaults to "standard" when empty or unknown.
//  2. For each tier, individual fields inherit the legacy LLM* field
//     when empty (field-by-field, not all-or-nothing). This allows an
//     operator to override just the model for one tier while keeping
//     everything else legacy.
//  3. LLMRoleDefaults is normalized: empty map is initialized; unknown
//     role names are dropped; tier values are lowercased and dropped if
//     not one of heavy/standard/light.
//
// EnsureLLMTierDefaults must run AFTER applyDefaults so that the legacy
// LLM* fields are themselves fully resolved (provider/model/base URL).
func EnsureLLMTierDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	cfg.LLMDefaultTier = normalizeTierName(cfg.LLMDefaultTier)
	if !isKnownTier(cfg.LLMDefaultTier) {
		cfg.LLMDefaultTier = TierStandard
	}

	seedTier(&cfg.LLMTierHeavy, cfg.LLMConfig)
	seedTier(&cfg.LLMTierStandard, cfg.LLMConfig)
	seedTier(&cfg.LLMTierLight, cfg.LLMConfig)

	cfg.LLMRoleDefaults = normalizeRoleDefaults(cfg.LLMRoleDefaults)
}

// seedTier fills empty fields of dst with values from the legacy LLM*
// configuration on cfg.
func seedTier(dst *LLMTierSettings, legacy LLMConfig) {
	if strings.TrimSpace(dst.Provider) == "" {
		dst.Provider = legacy.LLMProvider
	}
	if strings.TrimSpace(dst.AuthMode) == "" {
		dst.AuthMode = legacy.LLMAuthMode
	}
	if strings.TrimSpace(dst.OAuthProvider) == "" {
		dst.OAuthProvider = legacy.LLMOAuthProvider
	}
	if strings.TrimSpace(dst.BaseURL) == "" {
		dst.BaseURL = legacy.LLMBaseURL
	}
	if strings.TrimSpace(dst.APIKey) == "" {
		dst.APIKey = legacy.LLMAPIKey
	}
	if strings.TrimSpace(dst.Model) == "" {
		dst.Model = legacy.LLMModel
	}
	if strings.TrimSpace(dst.ReasoningEffort) == "" {
		dst.ReasoningEffort = legacy.LLMReasoningEffort
	}
	if dst.ThinkingBudget <= 0 {
		dst.ThinkingBudget = legacy.LLMThinkingBudget
	}
	if strings.TrimSpace(dst.ServiceTier) == "" {
		dst.ServiceTier = legacy.LLMServiceTier
	}
}

// tierSettingsAccessor is a helper type that lets field builders point at
// a specific tier's settings inside Config.
type tierSettingsAccessor func(*Config) *LLMTierSettings

// tierStringFieldSpec describes one string field inside a TierSettings
// struct (e.g. Provider, Model, ReasoningEffort).
type tierStringFieldSpec struct {
	accessor func(*LLMTierSettings) *string
	// normalize transforms the raw YAML/env value; pass identityString
	// to keep the value as-is.
	normalize func(string) string
}

// llmTierStringField builds a configInputField that writes a string
// subfield of an LLMTierSettings. The tier accessor picks which tier
// (heavy/standard/light) the field belongs to.
func llmTierStringField(yamlKey string, envKeys []string, tier tierSettingsAccessor, spec tierStringFieldSpec) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := spec.accessor(tier(cfg))
			*ptr = spec.normalize(raw)
		},
		merge: func(dst *Config, src Config) {
			value := *spec.accessor(tier(&src))
			if strings.TrimSpace(value) != "" {
				*spec.accessor(tier(dst)) = value
			}
		},
	}
}

// llmTierIntField is the int analogue of llmTierStringField. Used for
// ThinkingBudget. Only positive values override the legacy fallback.
func llmTierIntField(yamlKey string, envKeys []string, tier tierSettingsAccessor, accessor func(*LLMTierSettings) *int) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(tier(cfg))
			*ptr = parsePositiveInt(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(tier(&src))
			if value > 0 {
				*accessor(tier(dst)) = value
			}
		},
	}
}

// llmRoleField builds a configInputField that writes a single entry into
// the LLMRoleDefaults map. The role name is fixed at field-definition
// time; the YAML/env value supplies the tier name.
func llmRoleField(yamlKey, roleName string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			value := normalizeTierName(raw)
			if value == "" {
				return
			}
			if cfg.LLMRoleDefaults == nil {
				cfg.LLMRoleDefaults = make(map[string]string)
			}
			cfg.LLMRoleDefaults[roleName] = value
		},
		merge: func(dst *Config, src Config) {
			if src.LLMRoleDefaults == nil {
				return
			}
			value, ok := src.LLMRoleDefaults[roleName]
			if !ok || value == "" {
				return
			}
			if dst.LLMRoleDefaults == nil {
				dst.LLMRoleDefaults = make(map[string]string)
			}
			dst.LLMRoleDefaults[roleName] = value
		},
	}
}

// tierHeavy/tierStandard/tierLight are tierSettingsAccessor helpers.
func tierHeavyAccessor(cfg *Config) *LLMTierSettings    { return &cfg.LLMTierHeavy }
func tierStandardAccessor(cfg *Config) *LLMTierSettings { return &cfg.LLMTierStandard }
func tierLightAccessor(cfg *Config) *LLMTierSettings    { return &cfg.LLMTierLight }

// Spec factories for the nine per-tier string subfields. Centralizing
// them keeps field registration compact.
var (
	tierSpecProvider = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.Provider },
		normalize: identityString,
	}
	tierSpecAuthMode = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.AuthMode },
		normalize: identityString,
	}
	tierSpecOAuthProvider = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.OAuthProvider },
		normalize: identityString,
	}
	tierSpecBaseURL = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.BaseURL },
		normalize: identityString,
	}
	tierSpecAPIKey = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.APIKey },
		normalize: identityString,
	}
	tierSpecModel = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.Model },
		normalize: identityString,
	}
	tierSpecReasoningEffort = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.ReasoningEffort },
		normalize: identityString,
	}
	tierSpecServiceTier = tierStringFieldSpec{
		accessor:  func(t *LLMTierSettings) *string { return &t.ServiceTier },
		normalize: identityString,
	}
	tierSpecThinkingBudget = func(t *LLMTierSettings) *int { return &t.ThinkingBudget }
)

// normalizeRoleDefaults returns a cleaned copy of the role→tier map:
// role names are lowercased/trimmed and dropped if not recognized, tier
// values are lowercased/trimmed and dropped if not one of heavy/standard/light.
func normalizeRoleDefaults(src map[string]string) map[string]string {
	known := make(map[string]struct{}, len(allKnownRoles()))
	for _, r := range allKnownRoles() {
		known[r] = struct{}{}
	}
	out := make(map[string]string, len(src))
	for role, tier := range src {
		role = strings.ToLower(strings.TrimSpace(role))
		if _, ok := known[role]; !ok {
			continue
		}
		tierNorm := normalizeTierName(tier)
		if !isKnownTier(tierNorm) {
			continue
		}
		out[role] = tierNorm
	}
	return out
}
