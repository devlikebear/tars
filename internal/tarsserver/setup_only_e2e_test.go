package tarsserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

// TestSetupOnlyE2E_WizardSaveCycle walks through the full degraded
// boot path and the wizard happy path:
//
//  1. buildBaseDeps succeeds without LLM config
//  2. buildLLMDeps fails on init_llm — recoverable
//  3. CLI downgrade leaves deps.LLMReady=false
//  4. buildAPIMux delegates to the setup-only mux
//  5. The wizard polls healthz / setup/status, sees needs_setup=true
//  6. The wizard PATCHes /v1/admin/config/values to write providers + tiers
//  7. After patch, /v1/setup/status reports needs_setup=false
//
// This is the integration regression target the rest of Phase 2's
// unit tests are written against.
func TestSetupOnlyE2E_WizardSaveCycle(t *testing.T) {
	dir := t.TempDir()
	workspaceDir := filepath.Join(dir, "workspace")
	configPath := filepath.Join(dir, "config.yaml")

	// Seed a minimal config with no LLM section — the wizard's job to fill.
	if err := os.WriteFile(configPath, []byte("runtime:\n  workspace_dir: "+workspaceDir+"\n"), 0o600); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.WorkspaceDir = workspaceDir
	cfg.APIAuthMode = "off"
	cfg.APIAllowInsecureLocalAuth = true

	logger := zerolog.New(io.Discard)
	opts := &options{ConfigPath: configPath, APIAddr: "127.0.0.1:0"}

	base, err := buildBaseDeps(opts, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps: %v", err)
	}
	if base.LLMReady {
		t.Fatalf("buildBaseDeps must not set LLMReady")
	}

	_, llmErr := buildLLMDeps(base, cfg, logger)
	if llmErr == nil {
		t.Fatalf("expected buildLLMDeps to fail with empty cfg")
	}
	if !isRecoverableLLMInitError(llmErr) {
		t.Fatalf("expected recoverable init error, got %v", llmErr)
	}
	var depErr *runtimeDepsError
	if !errors.As(llmErr, &depErr) || depErr.stage != "init_llm" {
		t.Fatalf("expected init_llm stage, got %+v", depErr)
	}

	// Apply the same downgrade the CLI's RunE performs.
	deps := base
	deps.LLMReady = false

	apiRuntime, err := buildAPIMux(opts, deps, time.Now, logger, io.Discard)
	if err != nil {
		t.Fatalf("buildAPIMux setup-only: %v", err)
	}
	srv := apiRuntime.server.Handler

	// 1. healthz signals needs_setup=true.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var healthz struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &healthz); err != nil || !healthz.NeedsSetup {
		t.Fatalf("healthz needs_setup=true expected, body=%s err=%v", rec.Body.String(), err)
	}

	// 2. /v1/setup/status reports the missing pieces.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup/status expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var status struct {
		NeedsSetup bool `json:"needs_setup"`
		Providers  struct {
			Missing bool `json:"missing"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode setup/status: %v", err)
	}
	if !status.NeedsSetup || !status.Providers.Missing {
		t.Fatalf("expected needs_setup=true and providers.missing=true, body=%s", rec.Body.String())
	}

	// 3. /v1/chat is blocked with the setup-only fallback (503).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/v1/chat expected 503, got %d body=%q", rec.Code, rec.Body.String())
	}

	// 4. Wizard saves a provider + 3 tiers via PATCH /v1/admin/config/values.
	patch := map[string]any{
		"updates": map[string]any{
			"llm_providers": map[string]any{
				"openai": map[string]any{
					"kind":      "openai",
					"auth_mode": "api-key",
					"api_key":   "sk-test-fake-key",
					"base_url":  "https://api.openai.com/v1",
				},
			},
			"llm_tiers": map[string]any{
				"heavy":    map[string]any{"provider": "openai", "model": "gpt-5.4"},
				"standard": map[string]any{"provider": "openai", "model": "gpt-5.4"},
				"light":    map[string]any{"provider": "openai", "model": "gpt-5.4-mini"},
			},
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/v1/admin/config/values", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config/values PATCH expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	// 5. /v1/setup/status now reflects the persisted config (needs_setup=false).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post-patch setup/status expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode post-patch setup/status: %v", err)
	}
	if status.NeedsSetup || status.Providers.Missing {
		t.Fatalf("after wizard save expected needs_setup=false providers.missing=false, body=%s", rec.Body.String())
	}
}

// TestSetupOnlyE2E_MissingConfigFile_FirstInstall mirrors the first-
// install scenario: the operator never ran tars init, so neither the
// config file nor its parent directory exist. Booting must NOT panic
// and the wizard's first PATCH must succeed (creating the parent dir
// + writing the file in place).
func TestSetupOnlyE2E_MissingConfigFile_FirstInstall(t *testing.T) {
	dir := t.TempDir()
	workspaceDir := filepath.Join(dir, "workspace")
	// Path's parent does NOT exist — simulates ~/.tars/config/ on a
	// brand-new install. PatchYAML must mkdir it.
	configPath := filepath.Join(dir, "fresh-install", "tars", "config.yaml")
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("expected parent dir to be missing before boot, stat err=%v", err)
	}

	opts := &options{ConfigPath: configPath, APIAddr: "127.0.0.1:0", WorkspaceDir: workspaceDir}
	cfg, err := loadConfigForServe(opts)
	if err != nil {
		t.Fatalf("loadConfigForServe with missing file: %v", err)
	}
	cfg.APIAuthMode = "off"
	cfg.APIAllowInsecureLocalAuth = true

	logger := zerolog.New(io.Discard)
	base, err := buildBaseDeps(opts, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps: %v", err)
	}

	// LLM init fails recoverably on the empty cfg — exactly the
	// downgrade path the CLI's RunE follows.
	if _, llmErr := buildLLMDeps(base, cfg, logger); llmErr == nil {
		t.Fatalf("expected buildLLMDeps to fail on empty cfg")
	} else if !isRecoverableLLMInitError(llmErr) {
		t.Fatalf("expected recoverable error, got %v", llmErr)
	}

	deps := base
	deps.LLMReady = false

	apiRuntime, err := buildAPIMux(opts, deps, time.Now, logger, io.Discard)
	if err != nil {
		t.Fatalf("buildAPIMux setup-only: %v", err)
	}
	srv := apiRuntime.server.Handler

	// Wizard PATCH lands at the missing-parent path.
	patch := map[string]any{
		"updates": map[string]any{
			"llm_providers": map[string]any{
				"openai": map[string]any{
					"kind":      "openai",
					"auth_mode": "api-key",
					"api_key":   "sk-test-fake-key",
					"base_url":  "https://api.openai.com/v1",
				},
			},
			"llm_tiers": map[string]any{
				"heavy":    map[string]any{"provider": "openai", "model": "gpt-5.4"},
				"standard": map[string]any{"provider": "openai", "model": "gpt-5.4"},
				"light":    map[string]any{"provider": "openai", "model": "gpt-5.4-mini"},
			},
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/admin/config/values", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config/values PATCH on missing parent expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	// File + parent dir now exist.
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file created at %s, got err=%v", configPath, err)
	}

	// Reload reflects the persisted bindings.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/setup/status", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post-patch setup/status expected 200, got %d", rec.Code)
	}
	var status struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode setup/status: %v", err)
	}
	if status.NeedsSetup {
		t.Fatalf("after wizard save on missing-file path, expected needs_setup=false, got body=%s", rec.Body.String())
	}
}
