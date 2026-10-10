package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestCheckDoctorInitiative(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	var disabled doctorReport
	checkDoctorInitiative(&disabled, config.Config{})
	if c := only(t, disabled); c.status != "ok" || c.detail != "disabled" {
		t.Fatalf("disabled = %+v", c)
	}

	var noBackend doctorReport
	checkDoctorInitiative(&noBackend, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}})
	if c := only(t, noBackend); c.status != "warn" || !strings.Contains(c.detail, "jev.base_url") {
		t.Fatalf("no backend = %+v", c)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"ok":{"noul":0.9}}}`))
	}))
	defer srv.Close()
	var ok doctorReport
	checkDoctorInitiative(&ok, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, ok); c.status != "ok" || !strings.Contains(c.detail, "shadow") || !strings.Contains(c.detail, "loopback") || !strings.Contains(c.detail, "backend jev") {
		t.Fatalf("reachable = %+v", c)
	}

	srv.Close()
	var down doctorReport
	checkDoctorInitiative(&down, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, down); c.status != "warn" || !strings.Contains(c.detail, "unreachable") {
		t.Fatalf("down = %+v", c)
	}
}

func TestCheckDoctorInitiativeLLMBackend(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	llmConfig := func(lightAlias string) config.Config {
		return config.Config{
			Initiative: config.InitiativeConfig{Enabled: true, Backend: "llm"},
			LLMConfig: config.LLMConfig{
				LLMDefaultTier:  "standard",
				LLMRoleDefaults: map[string]string{"initiative": "light"},
				LLMProviders: map[string]config.LLMProviderSettings{
					"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
					"other":  {Kind: "openai", BaseURL: "http://other.example", APIKey: "k2"},
				},
				LLMTiers: map[string]config.LLMTierBinding{
					"heavy":    {Provider: "shared", Model: "heavy-model"},
					"standard": {Provider: "shared", Model: "chat-model"},
					"light":    {Provider: lightAlias, Model: "light-model"},
				},
			},
		}
	}

	var same doctorReport
	checkDoctorInitiative(&same, llmConfig("shared"))
	if c := only(t, same); c.status != "ok" || !strings.Contains(c.detail, "backend llm") ||
		!strings.Contains(c.detail, "reads user text") {
		t.Fatalf("same provider = %+v", c)
	}

	var different doctorReport
	checkDoctorInitiative(&different, llmConfig("other"))
	if c := only(t, different); c.status != "ok" || !strings.Contains(c.detail, "metadata only") {
		t.Fatalf("different provider = %+v", c)
	}

	var missingTier doctorReport
	cfg := llmConfig("shared")
	cfg.LLMRoleDefaults["initiative"] = "nonexistent"
	checkDoctorInitiative(&missingTier, cfg)
	if c := only(t, missingTier); c.status != "warn" || !strings.Contains(c.detail, "not configured") {
		t.Fatalf("missing tier = %+v", c)
	}

	var antigravity doctorReport
	cfg2 := llmConfig("shared")
	cfg2.LLMProviders["shared"] = config.LLMProviderSettings{Kind: "antigravity-cli"}
	checkDoctorInitiative(&antigravity, cfg2)
	if c := only(t, antigravity); c.status != "warn" || !strings.Contains(c.detail, "antigravity-cli") {
		t.Fatalf("antigravity-cli = %+v", c)
	}
}
