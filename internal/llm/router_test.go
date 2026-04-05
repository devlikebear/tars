package llm

import (
	"context"
	"testing"
)

func buildValidRouterConfig(defaultTier Tier, roleDefaults map[Role]Tier) RouterConfig {
	return RouterConfig{
		Tiers: map[Tier]TierEntry{
			TierHeavy:    {Client: &FakeClient{Label: "heavy"}, Provider: "anthropic", Model: "claude-opus-4-6"},
			TierStandard: {Client: &FakeClient{Label: "standard"}, Provider: "anthropic", Model: "claude-sonnet-4-6"},
			TierLight:    {Client: &FakeClient{Label: "light"}, Provider: "anthropic", Model: "claude-haiku-4-5"},
		},
		DefaultTier:  defaultTier,
		RoleDefaults: roleDefaults,
	}
}

func TestNewRouterValidConfig(t *testing.T) {
	router, err := NewRouter(buildValidRouterConfig(TierStandard, map[Role]Tier{
		RolePulseDecider:     TierLight,
		RoleReflectionMemory: TierLight,
		RoleGatewayPlanner:   TierHeavy,
	}))
	if err != nil {
		t.Fatalf("NewRouter returned error: %v", err)
	}
	if router.DefaultTier() != TierStandard {
		t.Errorf("DefaultTier = %q, want %q", router.DefaultTier(), TierStandard)
	}
}

func TestNewRouterRejectsEmptyDefaultTier(t *testing.T) {
	cfg := buildValidRouterConfig("", nil)
	if _, err := NewRouter(cfg); err == nil {
		t.Fatal("expected error for empty DefaultTier")
	}
}

func TestNewRouterRejectsUnknownDefaultTier(t *testing.T) {
	cfg := buildValidRouterConfig(Tier("ultra"), nil)
	if _, err := NewRouter(cfg); err == nil {
		t.Fatal("expected error for unknown DefaultTier")
	}
}

func TestNewRouterRejectsMissingTier(t *testing.T) {
	cfg := RouterConfig{
		Tiers: map[Tier]TierEntry{
			TierHeavy:    {Client: &FakeClient{Label: "heavy"}, Provider: "anthropic", Model: "opus"},
			TierStandard: {Client: &FakeClient{Label: "standard"}, Provider: "anthropic", Model: "sonnet"},
			// TierLight missing
		},
		DefaultTier: TierStandard,
	}
	if _, err := NewRouter(cfg); err == nil {
		t.Fatal("expected error for missing tier")
	}
}

func TestNewRouterRejectsNilClient(t *testing.T) {
	cfg := buildValidRouterConfig(TierStandard, nil)
	cfg.Tiers[TierLight] = TierEntry{Client: nil, Provider: "x", Model: "y"}
	if _, err := NewRouter(cfg); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestNewRouterRejectsRoleDefaultMappingToUnknownTier(t *testing.T) {
	cfg := buildValidRouterConfig(TierStandard, map[Role]Tier{
		RolePulseDecider: Tier("featherweight"),
	})
	if _, err := NewRouter(cfg); err == nil {
		t.Fatal("expected error for role mapping to unknown tier")
	}
}

func TestClientForRoleFallbackToDefault(t *testing.T) {
	router, err := NewRouter(buildValidRouterConfig(TierStandard, nil))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	_, res, err := router.ClientFor(RoleChatMain)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if res.Tier != TierStandard {
		t.Errorf("Tier = %q, want %q (default)", res.Tier, TierStandard)
	}
	if res.Source != "default" {
		t.Errorf("Source = %q, want %q", res.Source, "default")
	}
	if res.Role != RoleChatMain {
		t.Errorf("Role = %q, want %q", res.Role, RoleChatMain)
	}
}

func TestClientForRoleUsesRoleDefault(t *testing.T) {
	router, _, err := NewFakeRouter(TierStandard, map[Role]Tier{
		RolePulseDecider:     TierLight,
		RoleReflectionMemory: TierLight,
		RoleGatewayPlanner:   TierHeavy,
	})
	if err != nil {
		t.Fatalf("NewFakeRouter: %v", err)
	}
	// pulse_decider → light
	_, res, err := router.ClientFor(RolePulseDecider)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if res.Tier != TierLight {
		t.Errorf("pulse_decider Tier = %q, want light", res.Tier)
	}
	if res.Source != "role" {
		t.Errorf("Source = %q, want role", res.Source)
	}
	// gateway_planner → heavy
	_, res, err = router.ClientFor(RoleGatewayPlanner)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if res.Tier != TierHeavy {
		t.Errorf("gateway_planner Tier = %q, want heavy", res.Tier)
	}
}

func TestClientForTierExplicit(t *testing.T) {
	router, _, err := NewFakeRouter(TierStandard, nil)
	if err != nil {
		t.Fatalf("NewFakeRouter: %v", err)
	}
	_, res, err := router.ClientForTier(TierHeavy)
	if err != nil {
		t.Fatalf("ClientForTier: %v", err)
	}
	if res.Tier != TierHeavy {
		t.Errorf("Tier = %q, want heavy", res.Tier)
	}
	if res.Source != "explicit" {
		t.Errorf("Source = %q, want explicit", res.Source)
	}
}

func TestClientForTierRejectsInvalid(t *testing.T) {
	router, _, err := NewFakeRouter(TierStandard, nil)
	if err != nil {
		t.Fatalf("NewFakeRouter: %v", err)
	}
	if _, _, err := router.ClientForTier(Tier("xyz")); err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestTierForRole(t *testing.T) {
	router, _, err := NewFakeRouter(TierStandard, map[Role]Tier{
		RolePulseDecider: TierLight,
	})
	if err != nil {
		t.Fatalf("NewFakeRouter: %v", err)
	}
	if got := router.TierForRole(RolePulseDecider); got != TierLight {
		t.Errorf("TierForRole(pulse_decider) = %q, want light", got)
	}
	if got := router.TierForRole(RoleChatMain); got != TierStandard {
		t.Errorf("TierForRole(chat_main) = %q, want standard (default)", got)
	}
}

func TestFakeRouterRouting(t *testing.T) {
	router, clients, err := NewFakeRouter(TierStandard, map[Role]Tier{
		RolePulseDecider: TierLight,
	})
	if err != nil {
		t.Fatalf("NewFakeRouter: %v", err)
	}
	client, _, err := router.ClientFor(RolePulseDecider)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if _, err := client.Ask(context.Background(), "hi"); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if clients[TierLight].AskCalls != 1 {
		t.Errorf("expected light AskCalls=1, got %d", clients[TierLight].AskCalls)
	}
	if clients[TierHeavy].AskCalls != 0 || clients[TierStandard].AskCalls != 0 {
		t.Error("unexpected calls on heavy/standard clients")
	}
}
