package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func newTestSetupHandler(t *testing.T, configPath string, cfg config.Config) http.Handler {
	t.Helper()
	logger := zerolog.New(io.Discard)
	return newSetupAPIHandler(configPath, cfg, logger)
}

func TestSetupStatus_NeedsSetup_EmptyConfig(t *testing.T) {
	handler := newTestSetupHandler(t, "", config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatalf("expected needs_setup=true, body=%+v", body)
	}
	if body.Providers.Missing != true {
		t.Fatalf("expected providers.missing=true, body=%+v", body)
	}
	if body.Tiers["heavy"].Configured || body.Tiers["standard"].Configured || body.Tiers["light"].Configured {
		t.Fatalf("expected all tiers unconfigured, body=%+v", body.Tiers)
	}
	if !findCheck(t, body.Checks, "providers_present").OK == false {
		// providers_present should be false
		c := findCheck(t, body.Checks, "providers_present")
		if c.OK {
			t.Fatalf("expected providers_present check to be false, got %+v", c)
		}
	}
	if c := findCheck(t, body.Checks, "tiers_complete"); c.OK {
		t.Fatalf("expected tiers_complete check to be false, got %+v", c)
	}
}

func TestSetupStatus_NeedsSetup_PartialTiers(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"openai": {Kind: "openai", AuthMode: "api-key", APIKey: "sk-secret-12345678"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy": {Provider: "openai", Model: "gpt-5.4"},
		},
	}}
	handler := newTestSetupHandler(t, "", cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatalf("expected needs_setup=true (partial tiers), body=%+v", body)
	}
	if body.Providers.Missing {
		t.Fatalf("expected providers.missing=false (1 configured), body=%+v", body)
	}
	if len(body.Providers.Configured) != 1 || body.Providers.Configured[0] != "openai" {
		t.Fatalf("expected providers.configured=[openai], got %+v", body.Providers.Configured)
	}
	if !body.Tiers["heavy"].Configured {
		t.Fatalf("expected heavy tier configured, got %+v", body.Tiers["heavy"])
	}
	if body.Tiers["standard"].Configured || body.Tiers["light"].Configured {
		t.Fatalf("expected standard/light unconfigured, got %+v", body.Tiers)
	}
	if c := findCheck(t, body.Checks, "tiers_complete"); c.OK {
		t.Fatalf("expected tiers_complete=false, got %+v", c)
	}
	if !strings.Contains(strings.ToLower(findCheck(t, body.Checks, "tiers_complete").Message), "standard") {
		t.Fatalf("expected tiers_complete message to mention missing tiers, got %+v", findCheck(t, body.Checks, "tiers_complete"))
	}
}

func TestSetupStatus_Complete(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"openai": {Kind: "openai", AuthMode: "api-key", APIKey: "sk-secret-12345678"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy":    {Provider: "openai", Model: "gpt-5.4"},
			"standard": {Provider: "openai", Model: "gpt-5.4"},
			"light":    {Provider: "openai", Model: "gpt-5.4-mini"},
		},
	}}
	handler := newTestSetupHandler(t, "", cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.NeedsSetup {
		t.Fatalf("expected needs_setup=false on complete config, body=%+v", body)
	}
	for _, tier := range []string{"heavy", "standard", "light"} {
		if !body.Tiers[tier].Configured {
			t.Fatalf("expected %s tier configured, got %+v", tier, body.Tiers[tier])
		}
	}
	for _, c := range body.Checks {
		if !c.OK {
			t.Fatalf("expected all checks ok on complete config, got %+v", body.Checks)
		}
	}
}

func TestSetupStatus_MasksAPIKeys(t *testing.T) {
	const secretKey = "sk-secret-abcdef1234567890"
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"openai": {Kind: "openai", AuthMode: "api-key", APIKey: secretKey},
		},
	}}
	handler := newTestSetupHandler(t, "", cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), secretKey) {
		t.Fatalf("response leaked api_key in plain text: %s", rec.Body.String())
	}
}

func TestSetupStatus_RejectsNonGET(t *testing.T) {
	handler := newTestSetupHandler(t, "", config.Config{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/v1/setup/status", nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405 for %s, got %d", method, rec.Code)
			}
		})
	}
}

func TestSetupStatus_ConfigPathReports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{"openai": {Kind: "openai"}},
	}}
	handler := newTestSetupHandler(t, path, cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ConfigPath != path {
		t.Fatalf("expected config_path=%q, got %q", path, body.ConfigPath)
	}
	if body.ConfigExists {
		t.Fatalf("expected config_exists=false (file not created), got true")
	}
}

func findCheck(t *testing.T, checks []setupCheck, id string) setupCheck {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %q not found in %+v", id, checks)
	return setupCheck{}
}
