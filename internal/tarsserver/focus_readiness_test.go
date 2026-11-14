package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestChatProviderKind(t *testing.T) {
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"cli": {Kind: "claude-code-cli"},
				"api": {Kind: "anthropic", AuthMode: "api-key", APIKey: "x"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"heavy": {Provider: "cli", Model: "opus"},
				"light": {Provider: "api", Model: "claude-haiku-5-5"},
			},
			LLMDefaultTier: "light",
		},
	}
	if got := chatProviderKind(cfg); got != "anthropic" {
		t.Errorf("default tier: %q", got)
	}
	cfg.LLMRoleDefaults = map[string]string{"chat_main": "heavy"}
	if got := chatProviderKind(cfg); got != "claude-code-cli" {
		t.Errorf("chat role tier: %q", got)
	}
	if got := chatProviderAlias(cfg); got != "cli" {
		t.Errorf("alias: %q", got)
	}
	cfg.LLMRoleDefaults = map[string]string{"chat_main": "nope"}
	if got := chatProviderKind(cfg); got != "" {
		t.Errorf("unknown tier: %q", got)
	}
	if got := chatProviderKind(config.Config{}); got != "" {
		t.Errorf("no tiers: %q", got)
	}
}

func TestFindProviderCLI(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("CLAUDE_CODE_CLI_PATH", missing)
	t.Setenv("AGY_CLI_PATH", missing)
	t.Setenv("PATH", t.TempDir())
	for _, kind := range []string{"claude-code-cli", "antigravity-cli"} {
		if err := findProviderCLI(kind); err == nil {
			t.Errorf("%s: found a binary that is not there", kind)
		}
	}
	if err := findProviderCLI("anthropic"); err != nil {
		t.Errorf("an API provider runs no binary: %v", err)
	}
}

func TestFocusReadinessRoute(t *testing.T) {
	t.Setenv("TARS_FOCUS_GH_PATH", filepath.Join(t.TempDir(), "no-gh"))
	t.Setenv("PATH", t.TempDir()) // no tailscale either
	h := newFocusReadinessHandler(config.Config{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/focus/readiness", nil))
	var answer struct {
		Checks []struct{ ID, State string } `json:"checks"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &answer) != nil || len(answer.Checks) != 3 {
		t.Fatalf("readiness: %d %s", rec.Code, rec.Body.String())
	}
	want := map[string]string{"model": "missing", "gh": "missing", "tailscale": "missing"}
	for _, c := range answer.Checks {
		if want[c.ID] != c.State {
			t.Errorf("%s = %s, want %s", c.ID, c.State, want[c.ID])
		}
	}
}
