package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

// testRouterForClient wraps a single llm.Client in a three-tier router
// where every tier resolves to that client. Used by tests that exercise
// router-aware handlers without caring about tier selection.
func testRouterForClient(t *testing.T, client llm.Client) llm.Router {
	t.Helper()
	entry := llm.TierEntry{Client: client, Provider: "fake", Model: "fake-model"}
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    entry,
			llm.TierStandard: entry,
			llm.TierLight:    entry,
		},
		DefaultTier: llm.TierLight,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RoleContextCompactor: llm.TierLight,
			llm.RoleChatMain:         llm.TierStandard,
		},
	})
	if err != nil {
		t.Fatalf("build test router: %v", err)
	}
	return router
}
