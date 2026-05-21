package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

type fakeClient struct{}

func (fakeClient) Ask(context.Context, string) (string, error) {
	return "ok", nil
}

func TestPublicHelpers(t *testing.T) {
	if llm.ToolChoiceAuto().Mode != llm.ToolChoiceModeAuto {
		t.Fatalf("ToolChoiceAuto() did not return auto")
	}
	if llm.ToolChoiceNone().Mode != llm.ToolChoiceModeNone {
		t.Fatalf("ToolChoiceNone() did not return none")
	}
	if llm.ToolChoiceRequired().Mode != llm.ToolChoiceModeRequired {
		t.Fatalf("ToolChoiceRequired() did not return required")
	}
	if got := llm.ToolChoiceSpecific("echo"); got.Mode != llm.ToolChoiceModeSpecific || got.Name != "echo" {
		t.Fatalf("ToolChoiceSpecific() = %+v", got)
	}
	if llm.DefaultClientConfig().HTTPTimeout <= 0 {
		t.Fatalf("DefaultClientConfig() has no timeout")
	}
	if len(llm.AllRoles()) == 0 {
		t.Fatalf("AllRoles() is empty")
	}
	if role, ok := llm.ParseRole("memory_hook"); !ok || role != llm.RoleMemoryHook {
		t.Fatalf("ParseRole() = %q, %v", role, ok)
	}
	if len(llm.AllTiers()) != 3 {
		t.Fatalf("AllTiers() = %+v", llm.AllTiers())
	}
	if tier, err := llm.ParseTier("light"); err != nil || tier != llm.TierLight {
		t.Fatalf("ParseTier() = %q, %v", tier, err)
	}
	if rec := llm.RecommendTierForTask("draft an architecture migration plan"); rec.RecommendedTier == "" {
		t.Fatalf("RecommendTierForTask() = %+v", rec)
	}
	if _, err := llm.NewProvider(llm.ProviderOptions{Provider: "not-a-provider"}); err == nil {
		t.Fatalf("NewProvider() expected error for unknown provider")
	}
	_, _ = llm.FindClaudeCodeCLIPath()
	_, err := llm.NewClaudeCodeCLIClient(t.TempDir(), "sonnet")
	if err != nil && !strings.Contains(err.Error(), "claude") {
		t.Fatalf("NewClaudeCodeCLIClient() unexpected error = %v", err)
	}
}

func (fakeClient) Chat(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}, nil
}

func TestRouterUsesPublicTypes(t *testing.T) {
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    {Client: fakeClient{}, Provider: "fake", Model: "heavy"},
			llm.TierStandard: {Client: fakeClient{}, Provider: "fake", Model: "standard"},
			llm.TierLight:    {Client: fakeClient{}, Provider: "fake", Model: "light"},
		},
		DefaultTier: llm.TierStandard,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RoleMemoryHook: llm.TierLight,
		},
	})
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	_, resolution, err := router.ClientFor(llm.RoleMemoryHook)
	if err != nil {
		t.Fatalf("ClientFor() error = %v", err)
	}
	if resolution.Tier != llm.TierLight || resolution.Provider != "fake" {
		t.Fatalf("resolution = %+v", resolution)
	}
}
