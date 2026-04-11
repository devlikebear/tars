package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

// newRouterTestTracker builds a minimal in-memory usage.Tracker so that
// buildLLMRouter can wrap each tier client with NewTrackedClient. The
// returned tracker is not used for assertions; the tests only care
// about router wiring and per-tier metadata.
func newRouterTestTracker(t *testing.T) *usage.Tracker {
	t.Helper()
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	return tracker
}

// newAnthropicPoolCfg is a test helper that builds a Config with a single
// anthropic-kind provider and a map of tier→model bindings. Using the
// anthropic kind lets llm.NewProvider construct a client with just an
// API key, so we avoid OAuth paths that need external auth state.
func newAnthropicPoolCfg(aliasToKey map[string]string, tiers map[string]config.LLMTierBinding, defaultTier string, roleDefaults map[string]string) config.Config {
	providers := make(map[string]config.LLMProviderSettings, len(aliasToKey))
	for alias, key := range aliasToKey {
		providers[alias] = config.LLMProviderSettings{
			Kind:     "anthropic",
			AuthMode: "api-key",
			APIKey:   key,
			BaseURL:  "https://api.anthropic.com",
		}
	}
	return config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders:    providers,
			LLMTiers:        tiers,
			LLMDefaultTier:  defaultTier,
			LLMRoleDefaults: roleDefaults,
		},
	}
}

// --------------------------------------------------------------------
// Happy path
// --------------------------------------------------------------------

func TestBuildLLMRouter_SingleProviderSharedAcrossTiers(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant-shared"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6", ReasoningEffort: "high"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6", ReasoningEffort: "medium"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5", ReasoningEffort: "minimal"},
		},
		"standard",
		nil,
	)

	router, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err != nil {
		t.Fatalf("buildLLMRouter: %v", err)
	}
	if router == nil {
		t.Fatal("expected non-nil router")
	}
	if router.DefaultTier() != llm.TierStandard {
		t.Errorf("DefaultTier = %q, want standard", router.DefaultTier())
	}

	// Every tier should resolve through ClientForTier with correct metadata.
	for _, tc := range []struct {
		tier      llm.Tier
		wantModel string
	}{
		{llm.TierHeavy, "claude-opus-4-6"},
		{llm.TierStandard, "claude-sonnet-4-6"},
		{llm.TierLight, "claude-haiku-4-5"},
	} {
		client, resolution, err := router.ClientForTier(tc.tier)
		if err != nil {
			t.Errorf("ClientForTier %q: %v", tc.tier, err)
			continue
		}
		if client == nil {
			t.Errorf("ClientForTier %q: nil client", tc.tier)
		}
		if resolution.Provider != "anthropic" {
			t.Errorf("%s: Provider = %q, want anthropic", tc.tier, resolution.Provider)
		}
		if resolution.Model != tc.wantModel {
			t.Errorf("%s: Model = %q, want %q", tc.tier, resolution.Model, tc.wantModel)
		}
	}
}

// --------------------------------------------------------------------
// Multi-provider mix
// --------------------------------------------------------------------

func TestBuildLLMRouter_MultiProviderMix(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{
			"primary":   "sk-ant-primary",
			"secondary": "sk-ant-secondary",
		},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "primary", Model: "claude-opus-4-6"},
			"standard": {Provider: "primary", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "secondary", Model: "claude-haiku-4-5"},
		},
		"standard",
		nil,
	)

	router, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err != nil {
		t.Fatalf("buildLLMRouter: %v", err)
	}

	// Heavy and standard share alias primary; light uses secondary. The
	// router exposes Provider (the Kind) in the resolution, so all three
	// show "anthropic" — but the underlying clients are independent
	// objects (different BaseURL/APIKey would prove this more visibly;
	// we settle for a client-is-non-nil smoke here).
	for _, tc := range []struct {
		tier      llm.Tier
		wantModel string
	}{
		{llm.TierHeavy, "claude-opus-4-6"},
		{llm.TierStandard, "claude-sonnet-4-6"},
		{llm.TierLight, "claude-haiku-4-5"},
	} {
		client, resolution, err := router.ClientForTier(tc.tier)
		if err != nil {
			t.Errorf("ClientForTier %q: %v", tc.tier, err)
			continue
		}
		if client == nil {
			t.Errorf("%s: nil client", tc.tier)
		}
		if resolution.Model != tc.wantModel {
			t.Errorf("%s: Model = %q, want %q", tc.tier, resolution.Model, tc.wantModel)
		}
	}
}

// --------------------------------------------------------------------
// Role defaults propagation
// --------------------------------------------------------------------

func TestBuildLLMRouter_RoleDefaultsPropagated(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5"},
		},
		"standard",
		map[string]string{
			"chat_main":       "standard",
			"pulse_decider":   "light",
			"gateway_planner": "heavy",
		},
	)

	router, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err != nil {
		t.Fatalf("buildLLMRouter: %v", err)
	}

	for _, tc := range []struct {
		role     llm.Role
		wantTier llm.Tier
	}{
		{llm.RoleChatMain, llm.TierStandard},
		{llm.RolePulseDecider, llm.TierLight},
		{llm.RoleGatewayPlanner, llm.TierHeavy},
		// Unmapped role → default tier
		{llm.RoleContextCompactor, llm.TierStandard},
	} {
		if got := router.TierForRole(tc.role); got != tc.wantTier {
			t.Errorf("TierForRole(%q) = %q, want %q", tc.role, got, tc.wantTier)
		}
	}
}

func TestBuildLLMRouter_UnknownRoleInRoleDefaultsSilentlyDropped(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5"},
		},
		"standard",
		map[string]string{
			"chat_main":     "standard",
			"made_up_role":  "heavy", // unknown → silently dropped
			"pulse_decider": "light",
		},
	)

	router, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err != nil {
		t.Fatalf("buildLLMRouter: %v", err)
	}
	// Known roles are preserved; unknown roles fall back to default.
	if got := router.TierForRole(llm.RoleChatMain); got != llm.TierStandard {
		t.Errorf("chat_main tier = %q, want standard", got)
	}
	if got := router.TierForRole(llm.RolePulseDecider); got != llm.TierLight {
		t.Errorf("pulse_decider tier = %q, want light", got)
	}
}

// --------------------------------------------------------------------
// Error paths
// --------------------------------------------------------------------

func TestBuildLLMRouter_UnknownProviderAliasErrors(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "mistyped_alias", Model: "claude-haiku-4-5"},
		},
		"standard",
		nil,
	)

	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error for unknown provider alias, got nil")
	}
	if !strings.Contains(err.Error(), "mistyped_alias") {
		t.Errorf("error should mention alias name, got %q", err.Error())
	}
}

func TestBuildLLMRouter_EmptyModelErrors(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: ""}, // empty
			"light":    {Provider: "default", Model: "claude-haiku-4-5"},
		},
		"standard",
		nil,
	)

	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("error should mention model, got %q", err.Error())
	}
}

func TestBuildLLMRouter_MissingRequiredTierErrors(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			// "light" intentionally missing
		},
		"standard",
		nil,
	)

	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error for missing light tier, got nil")
	}
	// llm.NewRouter emits "tier \"light\" is missing from Tiers map"
	if !strings.Contains(err.Error(), "light") {
		t.Errorf("error should mention missing tier name, got %q", err.Error())
	}
}

func TestBuildLLMRouter_DefaultTierNotInTiersErrors(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5"},
		},
		"heavy", // valid key, but let's test a mismatch
		nil,
	)
	// Sanity: heavy is valid and should build successfully.
	if _, err := buildLLMRouter(cfg, newRouterTestTracker(t)); err != nil {
		t.Fatalf("buildLLMRouter with heavy default: %v", err)
	}

	// Now delete heavy from LLMTiers while keeping the binding — default
	// references a missing tier.
	cfg.LLMTiers = map[string]config.LLMTierBinding{
		"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
		"light":    {Provider: "default", Model: "claude-haiku-4-5"},
	}
	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error when default_tier references missing tier, got nil")
	}
}

func TestBuildLLMRouter_InvalidDefaultTierNameErrors(t *testing.T) {
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5"},
		},
		"banana", // not a known tier
		nil,
	)

	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error for invalid default_tier, got nil")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should mention the invalid tier name, got %q", err.Error())
	}
}

func TestBuildLLMRouter_EmptyPoolErrors(t *testing.T) {
	// Pool is nil — no providers defined at all. The tiers reference a
	// non-existent alias so resolution fails with a clear message.
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: nil,
			LLMTiers: map[string]config.LLMTierBinding{
				"heavy":    {Provider: "default", Model: "claude-opus-4-6"},
				"standard": {Provider: "default", Model: "claude-sonnet-4-6"},
				"light":    {Provider: "default", Model: "claude-haiku-4-5"},
			},
			LLMDefaultTier: "standard",
		},
	}

	_, err := buildLLMRouter(cfg, newRouterTestTracker(t))
	if err == nil {
		t.Fatal("expected error for empty pool, got nil")
	}
}
