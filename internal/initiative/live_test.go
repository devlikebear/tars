//go:build integration

package initiative

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/pkg/llm"
)

// TestInitiativeLightLiveTextSignals exercises the real configured
// initiative-role LLM (light tier by default) against a few Korean
// examples of the three atomic text signals — no mock server, no tool
// execution, no screen or driver. It never runs by default.
//
//	TARS_INITIATIVE_LIVE_CONFIG=<config> go test -tags integration ./internal/initiative -run TestInitiativeLightLiveTextSignals -v
func TestInitiativeLightLiveTextSignals(t *testing.T) {
	path := os.Getenv("TARS_INITIATIVE_LIVE_CONFIG")
	if path == "" {
		t.Skip("TARS_INITIATIVE_LIVE_CONFIG is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleInitiative)])
	if tier == "" {
		tier = "light"
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		t.Fatal(err)
	}
	if !llm.SupportsDecisionOnly(resolved.Kind) {
		t.Skipf("provider kind %q cannot run the initiative role (no decision-only isolation)", resolved.Kind)
	}
	client, err := llm.NewProvider(llm.ProviderOptions{
		Provider:        resolved.Kind,
		AuthMode:        resolved.AuthMode,
		OAuthProvider:   resolved.OAuthProvider,
		BaseURL:         resolved.BaseURL,
		WorkDir:         cfg.WorkspaceDir,
		Model:           resolved.Model,
		APIKey:          resolved.APIKey,
		ReasoningEffort: resolved.ReasoningEffort,
		ThinkingBudget:  resolved.ThinkingBudget,
		ServiceTier:     resolved.ServiceTier,
		MaxTokens:       resolved.MaxTokens,
		BetaFeatures:    resolved.BetaFeatures,
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := &LLMTextBackend{Client: client, Model: resolved.Model}
	today := time.Date(2026, 9, 29, 21, 0, 0, 0, time.UTC)
	questions := textQuestionsFor(today)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cases := []struct {
		name  string
		state string
		check func(TextAnswer) bool
	}{
		{
			name: "quiet request",
			state: "now: Tue 2026-09-29 21:00\nconsole: connected just now\nlast_user_message: just now\n" +
				"recent_user_messages (newest last):\n- \"오늘은 조용히 있고 싶어, 메시지 보내지 마\" (just now)",
			check: func(a TextAnswer) bool { return a.QuietRequested },
		},
		{
			name: "strained tone",
			state: "now: Tue 2026-09-29 21:00\nconsole: connected just now\nlast_user_message: just now\n" +
				"recent_user_messages (newest last):\n- \"너무 피곤하고 힘들어서 아무것도 하기 싫어\" (just now)",
			check: func(a TextAnswer) bool { return a.UserStrained },
		},
		{
			name:  "special day from profile",
			state: "now: Tue 2026-09-29 00:00\nconsole: not connected\nlast_user_message: none\nprofile:\nbirthday: September 29",
			check: func(a TextAnswer) bool { return a.SpecialDay },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := backend.ReadText(ctx, c.state, questions)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("model=%s state=%q got=%+v", resolved.Model, c.state, got)
			if !c.check(got) {
				t.Fatalf("expected signal not read: %+v", got)
			}
		})
	}
}
