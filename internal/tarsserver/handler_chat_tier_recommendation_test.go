package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

func TestResolveChatTierRecommendationFallsBackOnFirstTurn(t *testing.T) {
	rec, err := resolveChatTierRecommendation(nil, "Implement the next issue and verify the release.", true)
	if err != nil {
		t.Fatalf("resolveChatTierRecommendation: %v", err)
	}
	if rec.ChosenTier != llm.TierHeavy || rec.RecommendedTier != llm.TierHeavy {
		t.Fatalf("tier = chosen %q recommended %q, want heavy", rec.ChosenTier, rec.RecommendedTier)
	}
	if !rec.Accepted || rec.Source != "server" {
		t.Fatalf("fallback should be accepted server recommendation: %+v", rec)
	}
}

func TestResolveChatTierRecommendationCanDisableFallback(t *testing.T) {
	rec, err := resolveChatTierRecommendation(nil, "Implement the next issue and verify the release.", false)
	if err != nil {
		t.Fatalf("resolveChatTierRecommendation: %v", err)
	}
	if rec.enabled() {
		t.Fatalf("recommendation should be disabled for internal non-chat callers: %+v", rec)
	}
}

func TestResolveChatClientForTierUsesExplicitTier(t *testing.T) {
	entry := llm.TierEntry{Client: &llm.FakeClient{Label: "fake"}}
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    {Client: &llm.FakeClient{Label: "heavy"}, Model: "heavy-model", Provider: "fake"},
			llm.TierStandard: entry,
			llm.TierLight:    {Client: &llm.FakeClient{Label: "light"}, Model: "light-model", Provider: "fake"},
		},
		DefaultTier: llm.TierStandard,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RoleChatMain: llm.TierStandard,
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, resolution, err := (chatHandlerDeps{router: router}).resolveChatClientForTier("light")
	if err != nil {
		t.Fatalf("resolveChatClientForTier: %v", err)
	}
	if resolution.Tier != llm.TierLight || resolution.Source != "explicit" {
		t.Fatalf("resolution = %+v, want explicit light", resolution)
	}
}
