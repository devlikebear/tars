package config

import (
	"testing"
)

// TestEnsureLLMTierDefaults_LegacyOnly verifies that configs without any
// tier override continue to work by seeding all three tiers from the
// legacy llm_* fields.
func TestEnsureLLMTierDefaults_LegacyOnly(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProvider:        "anthropic",
			LLMAuthMode:        "api-key",
			LLMBaseURL:         "https://api.anthropic.com",
			LLMModel:           "claude-3-5-haiku-latest",
			LLMAPIKey:          "key-abc",
			LLMReasoningEffort: "medium",
			LLMThinkingBudget:  4096,
			LLMServiceTier:     "default",
		},
	}
	EnsureLLMTierDefaults(cfg)

	if cfg.LLMDefaultTier != TierStandard {
		t.Errorf("LLMDefaultTier = %q, want %q", cfg.LLMDefaultTier, TierStandard)
	}
	for _, tt := range []struct {
		name string
		got  LLMTierSettings
	}{
		{"heavy", cfg.LLMTierHeavy},
		{"standard", cfg.LLMTierStandard},
		{"light", cfg.LLMTierLight},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Provider != "anthropic" {
				t.Errorf("Provider = %q, want anthropic", tt.got.Provider)
			}
			if tt.got.Model != "claude-3-5-haiku-latest" {
				t.Errorf("Model = %q, want claude-3-5-haiku-latest", tt.got.Model)
			}
			if tt.got.APIKey != "key-abc" {
				t.Errorf("APIKey = %q, want key-abc", tt.got.APIKey)
			}
			if tt.got.BaseURL != "https://api.anthropic.com" {
				t.Errorf("BaseURL = %q", tt.got.BaseURL)
			}
			if tt.got.ReasoningEffort != "medium" {
				t.Errorf("ReasoningEffort = %q", tt.got.ReasoningEffort)
			}
			if tt.got.ThinkingBudget != 4096 {
				t.Errorf("ThinkingBudget = %d", tt.got.ThinkingBudget)
			}
			if tt.got.ServiceTier != "default" {
				t.Errorf("ServiceTier = %q", tt.got.ServiceTier)
			}
		})
	}
}

// TestEnsureLLMTierDefaults_TierOverride verifies that explicit tier
// fields win over the legacy values on a field-by-field basis.
func TestEnsureLLMTierDefaults_TierOverride(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProvider: "anthropic",
			LLMModel:    "claude-3-5-haiku-latest",
			LLMAPIKey:   "legacy-key",
		},
	}
	cfg.LLMTierHeavy = LLMTierSettings{
		Model:           "claude-opus-4-6",
		ReasoningEffort: "high",
	}
	cfg.LLMTierLight = LLMTierSettings{
		Model: "claude-haiku-4-5-20251001",
	}
	EnsureLLMTierDefaults(cfg)

	if cfg.LLMTierHeavy.Model != "claude-opus-4-6" {
		t.Errorf("heavy.Model = %q, want claude-opus-4-6", cfg.LLMTierHeavy.Model)
	}
	if cfg.LLMTierHeavy.Provider != "anthropic" {
		t.Errorf("heavy.Provider = %q, want anthropic (inherited)", cfg.LLMTierHeavy.Provider)
	}
	if cfg.LLMTierHeavy.ReasoningEffort != "high" {
		t.Errorf("heavy.ReasoningEffort = %q, want high", cfg.LLMTierHeavy.ReasoningEffort)
	}
	if cfg.LLMTierHeavy.APIKey != "legacy-key" {
		t.Errorf("heavy.APIKey = %q, want legacy-key", cfg.LLMTierHeavy.APIKey)
	}
	if cfg.LLMTierStandard.Model != "claude-3-5-haiku-latest" {
		t.Errorf("standard.Model = %q, want legacy fallback", cfg.LLMTierStandard.Model)
	}
	if cfg.LLMTierLight.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("light.Model = %q", cfg.LLMTierLight.Model)
	}
}

// TestEnsureLLMTierDefaults_DefaultTier verifies the default tier
// normalization and fallback.
func TestEnsureLLMTierDefaults_DefaultTier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty falls back to standard", "", TierStandard},
		{"unknown falls back to standard", "super", TierStandard},
		{"heavy accepted", "heavy", TierHeavy},
		{"light accepted", "light", TierLight},
		{"uppercase normalized", "HEAVY", TierHeavy},
		{"padded normalized", "  light  ", TierLight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				LLMConfig: LLMConfig{
					LLMProvider:    "anthropic",
					LLMModel:       "m",
					LLMDefaultTier: tt.input,
				},
			}
			EnsureLLMTierDefaults(cfg)
			if cfg.LLMDefaultTier != tt.expected {
				t.Errorf("LLMDefaultTier = %q, want %q", cfg.LLMDefaultTier, tt.expected)
			}
		})
	}
}

// TestEnsureLLMTierDefaults_RoleDefaultsNormalization verifies that
// unknown roles and unknown tier values are dropped from the role map.
func TestEnsureLLMTierDefaults_RoleDefaultsNormalization(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProvider: "anthropic",
			LLMModel:    "m",
			LLMRoleDefaults: map[string]string{
				RoleChatMain:       "heavy",
				RolePulseDecider:   "LIGHT",
				"bogus_role":       "heavy",
				RoleMemoryHook:     "ultra",
				RoleGatewayPlanner: "  heavy  ",
			},
		},
	}
	EnsureLLMTierDefaults(cfg)

	if len(cfg.LLMRoleDefaults) != 3 {
		t.Errorf("len(LLMRoleDefaults) = %d, want 3", len(cfg.LLMRoleDefaults))
	}
	if cfg.LLMRoleDefaults[RoleChatMain] != TierHeavy {
		t.Errorf("chat_main = %q, want heavy", cfg.LLMRoleDefaults[RoleChatMain])
	}
	if cfg.LLMRoleDefaults[RolePulseDecider] != TierLight {
		t.Errorf("pulse_decider = %q, want light", cfg.LLMRoleDefaults[RolePulseDecider])
	}
	if cfg.LLMRoleDefaults[RoleGatewayPlanner] != TierHeavy {
		t.Errorf("gateway_planner = %q, want heavy", cfg.LLMRoleDefaults[RoleGatewayPlanner])
	}
	if _, ok := cfg.LLMRoleDefaults["bogus_role"]; ok {
		t.Error("bogus_role should have been dropped")
	}
	if _, ok := cfg.LLMRoleDefaults[RoleMemoryHook]; ok {
		t.Error("memory_hook with invalid tier should have been dropped")
	}
}

// TestEnsureLLMTierDefaults_NilSafe verifies no panic on nil.
func TestEnsureLLMTierDefaults_NilSafe(t *testing.T) {
	EnsureLLMTierDefaults(nil) // must not panic
}

// TestEnsureLLMTierDefaults_IdempotencyAndApplyDefaults verifies that
// running the full applyDefaults pipeline twice produces identical
// tier settings (no double-seeding).
func TestEnsureLLMTierDefaults_IdempotencyAndApplyDefaults(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProvider: "anthropic",
		},
	}
	applyDefaults(cfg)
	first := cfg.LLMTierStandard
	applyDefaults(cfg)
	second := cfg.LLMTierStandard
	if first != second {
		t.Errorf("applyDefaults not idempotent:\nfirst=%+v\nsecond=%+v", first, second)
	}
}
