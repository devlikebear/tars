package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --------------------------------------------------------------------
// parseLLMProvidersJSON
// --------------------------------------------------------------------

func TestParseLLMProvidersJSON_ValidInput(t *testing.T) {
	raw := `{
		"codex": {
			"kind": "openai-codex",
			"auth_mode": "oauth",
			"oauth_provider": "openai-codex",
			"base_url": "https://chatgpt.com/backend-api",
			"service_tier": "priority"
		},
		"anthropic_direct": {
			"kind": "anthropic",
			"auth_mode": "api-key",
			"api_key": "sk-ant-literal",
			"base_url": "https://api.anthropic.com"
		}
	}`

	got := parseLLMProvidersJSON(raw, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}

	codex, ok := got["codex"]
	if !ok {
		t.Fatal("missing codex alias")
	}
	if codex.Kind != "openai-codex" {
		t.Errorf("codex.Kind = %q, want openai-codex", codex.Kind)
	}
	if codex.AuthMode != "oauth" {
		t.Errorf("codex.AuthMode = %q, want oauth", codex.AuthMode)
	}
	if codex.BaseURL != "https://chatgpt.com/backend-api" {
		t.Errorf("codex.BaseURL = %q, want https://chatgpt.com/backend-api", codex.BaseURL)
	}
	if codex.ServiceTier != "priority" {
		t.Errorf("codex.ServiceTier = %q, want priority", codex.ServiceTier)
	}

	anthro, ok := got["anthropic_direct"]
	if !ok {
		t.Fatal("missing anthropic_direct alias")
	}
	if anthro.APIKey != "sk-ant-literal" {
		t.Errorf("anthropic_direct.APIKey = %q, want sk-ant-literal", anthro.APIKey)
	}
}

func TestParseLLMProvidersJSON_EnvVarExpansion(t *testing.T) {
	t.Setenv("TARS_TEST_PROVIDER_KEY", "sk-live-xyz-789")
	t.Setenv("TARS_TEST_PROVIDER_URL", "https://custom.example.com")

	raw := `{
		"live": {
			"kind": "anthropic",
			"auth_mode": "api-key",
			"api_key": "${TARS_TEST_PROVIDER_KEY}",
			"base_url": "${TARS_TEST_PROVIDER_URL}"
		}
	}`

	got := parseLLMProvidersJSON(raw, nil)
	live, ok := got["live"]
	if !ok {
		t.Fatal("missing live alias")
	}
	if live.APIKey != "sk-live-xyz-789" {
		t.Errorf("APIKey = %q, want sk-live-xyz-789 (env expanded)", live.APIKey)
	}
	if live.BaseURL != "https://custom.example.com" {
		t.Errorf("BaseURL = %q, want https://custom.example.com (env expanded)", live.BaseURL)
	}
}

func TestParseLLMProvidersJSON_MissingEnvVarBecomesEmpty(t *testing.T) {
	// Ensure the env var is NOT set.
	if err := os.Unsetenv("TARS_TEST_ABSENT_KEY"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}

	raw := `{"broken": {"kind": "anthropic", "api_key": "${TARS_TEST_ABSENT_KEY}"}}`
	got := parseLLMProvidersJSON(raw, nil)
	if got["broken"].APIKey != "" {
		t.Errorf("expected empty APIKey for missing env var, got %q", got["broken"].APIKey)
	}
}

func TestParseLLMProvidersJSON_EmptyInput_ReturnsFallback(t *testing.T) {
	fallback := map[string]LLMProviderSettings{
		"default": {Kind: "anthropic"},
	}
	got := parseLLMProvidersJSON("", fallback)
	if len(got) != 1 || got["default"].Kind != "anthropic" {
		t.Errorf("empty input should return fallback, got %+v", got)
	}
}

func TestParseLLMProvidersJSON_MalformedJSON_ReturnsFallback(t *testing.T) {
	fallback := map[string]LLMProviderSettings{
		"default": {Kind: "anthropic"},
	}
	got := parseLLMProvidersJSON("not valid json{{", fallback)
	if len(got) != 1 || got["default"].Kind != "anthropic" {
		t.Errorf("malformed JSON should return fallback, got %+v", got)
	}
}

func TestParseLLMProvidersJSON_EmptyAliasDropped(t *testing.T) {
	raw := `{"": {"kind": "anthropic"}, "   ": {"kind": "openai"}, "codex": {"kind": "openai-codex"}}`
	got := parseLLMProvidersJSON(raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after dropping empty aliases, got %d: %+v", len(got), got)
	}
	if _, ok := got["codex"]; !ok {
		t.Error("codex alias missing")
	}
}

// --------------------------------------------------------------------
// parseLLMTiersJSON
// --------------------------------------------------------------------

func TestParseLLMTiersJSON_ValidInput(t *testing.T) {
	raw := `{
		"heavy":    {"provider": "codex", "model": "gpt-5.4", "reasoning_effort": "high", "thinking_budget": 8000},
		"standard": {"provider": "codex", "model": "gpt-5.4", "reasoning_effort": "medium"},
		"light":    {"provider": "minimax", "model": "MiniMax-M2.7", "service_tier": "flex"}
	}`

	got := parseLLMTiersJSON(raw, nil)
	if len(got) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(got))
	}

	heavy := got["heavy"]
	if heavy.Provider != "codex" || heavy.Model != "gpt-5.4" {
		t.Errorf("heavy mismatch: %+v", heavy)
	}
	if heavy.ReasoningEffort != "high" || heavy.ThinkingBudget != 8000 {
		t.Errorf("heavy knobs mismatch: effort=%q budget=%d", heavy.ReasoningEffort, heavy.ThinkingBudget)
	}

	light := got["light"]
	if light.Provider != "minimax" || light.Model != "MiniMax-M2.7" || light.ServiceTier != "flex" {
		t.Errorf("light mismatch: %+v", light)
	}
}

func TestParseLLMTiersJSON_TierKeyLowercasing(t *testing.T) {
	raw := `{"HEAVY": {"provider": "codex", "model": "gpt-5.4"}, "  Standard  ": {"provider": "codex", "model": "gpt-5.4"}}`
	got := parseLLMTiersJSON(raw, nil)
	if _, ok := got["heavy"]; !ok {
		t.Error("HEAVY should be normalized to heavy")
	}
	if _, ok := got["standard"]; !ok {
		t.Error("'  Standard  ' should be normalized to standard")
	}
}

func TestParseLLMTiersJSON_EnvExpansion(t *testing.T) {
	t.Setenv("TARS_TEST_TIER_MODEL", "claude-opus-from-env")
	raw := `{"heavy": {"provider": "anthropic", "model": "${TARS_TEST_TIER_MODEL}"}}`
	got := parseLLMTiersJSON(raw, nil)
	if got["heavy"].Model != "claude-opus-from-env" {
		t.Errorf("Model env expansion failed, got %q", got["heavy"].Model)
	}
}

func TestParseLLMTiersJSON_MalformedReturnsFallback(t *testing.T) {
	fallback := map[string]LLMTierBinding{"heavy": {Provider: "x", Model: "y"}}
	got := parseLLMTiersJSON("{{", fallback)
	if len(got) != 1 || got["heavy"].Provider != "x" {
		t.Errorf("malformed input should return fallback, got %+v", got)
	}
}

// --------------------------------------------------------------------
// parseLLMRoleDefaultsJSON
// --------------------------------------------------------------------

func TestParseLLMRoleDefaultsJSON_ValidInput(t *testing.T) {
	raw := `{
		"chat_main": "standard",
		"pulse_decider": "light",
		"gateway_planner": "heavy"
	}`
	got := parseLLMRoleDefaultsJSON(raw, nil)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got["chat_main"] != "standard" {
		t.Errorf("chat_main = %q, want standard", got["chat_main"])
	}
}

func TestParseLLMRoleDefaultsJSON_NormalizesKeysAndValues(t *testing.T) {
	raw := `{"CHAT_MAIN": "STANDARD", "  Pulse_Decider  ": "  LIGHT  "}`
	got := parseLLMRoleDefaultsJSON(raw, nil)
	if got["chat_main"] != "standard" {
		t.Errorf("expected chat_main -> standard, got %q", got["chat_main"])
	}
	if got["pulse_decider"] != "light" {
		t.Errorf("expected pulse_decider -> light, got %q", got["pulse_decider"])
	}
}

func TestParseLLMRoleDefaultsJSON_EmptyPairsDropped(t *testing.T) {
	raw := `{"": "standard", "chat_main": "", "gateway_planner": "heavy"}`
	got := parseLLMRoleDefaultsJSON(raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(got), got)
	}
	if got["gateway_planner"] != "heavy" {
		t.Error("gateway_planner missing")
	}
}

// --------------------------------------------------------------------
// Field builder merge behavior
// --------------------------------------------------------------------

func TestLLMProvidersField_Merge(t *testing.T) {
	field := llmProvidersField("llm_providers", nil)

	dst := Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{"old": {Kind: "anthropic"}},
		},
	}
	src := Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{"new": {Kind: "openai"}},
		},
	}

	field.merge(&dst, src)

	if len(dst.LLMProviders) != 1 {
		t.Fatalf("expected 1 provider after merge, got %d", len(dst.LLMProviders))
	}
	if _, ok := dst.LLMProviders["new"]; !ok {
		t.Error("merged result missing 'new' provider")
	}
	if _, ok := dst.LLMProviders["old"]; ok {
		t.Error("merge should replace, not union")
	}

	// Src is empty → dst preserved
	preserved := Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{"keep": {Kind: "anthropic"}},
		},
	}
	field.merge(&preserved, Config{})
	if preserved.LLMProviders["keep"].Kind != "anthropic" {
		t.Error("merge with empty src should preserve dst")
	}
}

func TestLLMTiersField_Merge(t *testing.T) {
	field := llmTiersField("llm_tiers", nil)
	dst := Config{LLMConfig: LLMConfig{LLMTiers: map[string]LLMTierBinding{"heavy": {Provider: "old"}}}}
	src := Config{LLMConfig: LLMConfig{LLMTiers: map[string]LLMTierBinding{"heavy": {Provider: "new"}}}}

	field.merge(&dst, src)
	if dst.LLMTiers["heavy"].Provider != "new" {
		t.Errorf("heavy provider after merge = %q, want new", dst.LLMTiers["heavy"].Provider)
	}
}

func TestLLMRoleDefaultsField_Merge(t *testing.T) {
	field := llmRoleDefaultsField("llm_role_defaults", nil)
	dst := Config{LLMConfig: LLMConfig{LLMRoleDefaults: map[string]string{"chat_main": "heavy"}}}
	src := Config{LLMConfig: LLMConfig{LLMRoleDefaults: map[string]string{"chat_main": "standard"}}}

	field.merge(&dst, src)
	if dst.LLMRoleDefaults["chat_main"] != "standard" {
		t.Errorf("chat_main after merge = %q, want standard", dst.LLMRoleDefaults["chat_main"])
	}
}

// --------------------------------------------------------------------
// End-to-end: YAML loader integration
// --------------------------------------------------------------------

func TestLoadYAML_LLMProviderPoolRoundTrip(t *testing.T) {
	t.Setenv("TARS_TEST_ANTHROPIC_KEY", "sk-ant-from-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlBody := `
mode: standalone
workspace_dir: ./workspace

llm_providers:
  codex:
    kind: openai-codex
    auth_mode: oauth
    oauth_provider: openai-codex
    base_url: https://chatgpt.com/backend-api
  anthropic_direct:
    kind: anthropic
    auth_mode: api-key
    api_key: ${TARS_TEST_ANTHROPIC_KEY}
    base_url: https://api.anthropic.com

llm_tiers:
  heavy:
    provider: anthropic_direct
    model: claude-opus-4-6
    reasoning_effort: high
    thinking_budget: 8000
  standard:
    provider: codex
    model: gpt-5.4
    reasoning_effort: medium
  light:
    provider: codex
    model: gpt-5.4-mini
    reasoning_effort: minimal

llm_default_tier: standard

llm_role_defaults:
  chat_main: standard
  pulse_decider: light
  gateway_planner: heavy
`
	if err := os.WriteFile(path, []byte(yamlBody), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := loadYAML(path)
	if err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	// Providers parsed
	if len(cfg.LLMProviders) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.LLMProviders))
	}
	if cfg.LLMProviders["codex"].BaseURL != "https://chatgpt.com/backend-api" {
		t.Errorf("codex.BaseURL wrong: %q", cfg.LLMProviders["codex"].BaseURL)
	}
	if cfg.LLMProviders["anthropic_direct"].APIKey != "sk-ant-from-env" {
		t.Errorf("anthropic_direct.APIKey env expansion failed: %q", cfg.LLMProviders["anthropic_direct"].APIKey)
	}

	// Tiers parsed
	if len(cfg.LLMTiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(cfg.LLMTiers))
	}
	heavy := cfg.LLMTiers["heavy"]
	if heavy.Provider != "anthropic_direct" || heavy.Model != "claude-opus-4-6" {
		t.Errorf("heavy tier mismatch: %+v", heavy)
	}
	if heavy.ThinkingBudget != 8000 {
		t.Errorf("heavy ThinkingBudget = %d, want 8000", heavy.ThinkingBudget)
	}

	// Default tier (already existing field, should still work)
	if cfg.LLMDefaultTier != "standard" {
		t.Errorf("LLMDefaultTier = %q, want standard", cfg.LLMDefaultTier)
	}

	// Role defaults
	if cfg.LLMRoleDefaults["chat_main"] != "standard" {
		t.Errorf("chat_main = %q, want standard", cfg.LLMRoleDefaults["chat_main"])
	}
	if cfg.LLMRoleDefaults["gateway_planner"] != "heavy" {
		t.Errorf("gateway_planner = %q, want heavy", cfg.LLMRoleDefaults["gateway_planner"])
	}

	// End-to-end: resolver consumes the loaded config correctly
	resolved, err := ResolveAllLLMTiers(&cfg)
	if err != nil {
		t.Fatalf("ResolveAllLLMTiers: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("resolver produced %d tiers, want 3", len(resolved))
	}
	rHeavy := resolved["heavy"]
	if rHeavy.Kind != "anthropic" {
		t.Errorf("resolved heavy Kind = %q, want anthropic", rHeavy.Kind)
	}
	if rHeavy.APIKey != "sk-ant-from-env" {
		t.Errorf("resolved heavy APIKey = %q, want sk-ant-from-env", rHeavy.APIKey)
	}
	if rHeavy.Model != "claude-opus-4-6" {
		t.Errorf("resolved heavy Model = %q, want claude-opus-4-6", rHeavy.Model)
	}
	if rHeavy.ProviderAlias != "anthropic_direct" {
		t.Errorf("resolved heavy ProviderAlias = %q, want anthropic_direct", rHeavy.ProviderAlias)
	}
}

func TestLoadYAML_EnvOverrideJSONForProviders(t *testing.T) {
	// Simulate env override path: write a minimal yaml, then apply the
	// env var which should replace LLMProviders when applied through
	// configInputFieldByYAMLKey. We exercise the field directly here
	// since loadYAML does not read env (that is a separate pass).

	field, ok := configInputFieldByYAMLKey("llm_providers")
	if !ok {
		t.Fatal("llm_providers field not registered")
	}

	cfg := &Config{}
	rawJSON := `{"envd": {"kind": "openai", "api_key": "envd-key", "base_url": "https://envd.example"}}`
	field.apply(cfg, rawJSON)

	if len(cfg.LLMProviders) != 1 {
		t.Fatalf("expected 1 provider from env json, got %d", len(cfg.LLMProviders))
	}
	if cfg.LLMProviders["envd"].APIKey != "envd-key" {
		t.Errorf("envd.APIKey = %q, want envd-key", cfg.LLMProviders["envd"].APIKey)
	}
}
