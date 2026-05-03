package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
)

type fakeModelFetcher struct {
	calls   int
	models  []string
	err     error
	lastOps llm.ProviderOptions
}

// makePoolTestCfg builds a minimal Config for handler tests using the
// provider pool schema. The tests use a single "default" alias + a
// "standard" tier mapped to it; the handler reads the default tier's
// resolved view.
func makePoolTestCfg(kind, model, authMode, baseURL string) config.Config {
	return config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"default": {
					Kind:     kind,
					AuthMode: authMode,
					BaseURL:  baseURL,
				},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"standard": {Provider: "default", Model: model},
			},
			LLMDefaultTier: "standard",
		},
	}
}

func (f *fakeModelFetcher) FetchModels(_ context.Context, opts llm.ProviderOptions) ([]string, error) {
	f.calls++
	f.lastOps = opts
	if f.err != nil {
		return nil, f.err
	}
	return append([]string(nil), f.models...), nil
}

func TestModelCache_PathUnderAgentRuntimePersistenceDir_(t *testing.T) {
	cfg := config.Config{
		AgentRuntimeConfig: config.AgentRuntimeConfig{
			AgentRuntimePersistenceDir: filepath.Join(t.TempDir(), "agentruntime"),
		},
	}
	got := providerModelsCachePath(cfg)
	want := filepath.Join(cfg.AgentRuntimePersistenceDir, providerModelsCacheFile)
	if got != want {
		t.Fatalf("expected cache path %q, got %q", want, got)
	}
}

func TestProvidersAPI_ReturnsCurrentAndSupportedProviders(t *testing.T) {
	cfg := makePoolTestCfg("openai-codex", "gpt-5.3-codex", "oauth", "")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, time.Now)
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(cfg, cache, &fakeModelFetcher{}, time.Now)
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var out providersAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode providers response: %v", err)
	}
	if out.CurrentProvider != "openai-codex" || out.CurrentModel != "gpt-5.3-codex" || out.AuthMode != "oauth" {
		t.Fatalf("unexpected providers payload: %+v", out)
	}
		if len(out.Providers) != len(supportedLiveModelProviders) {
			t.Fatalf("expected %d providers, got %d", len(supportedLiveModelProviders), len(out.Providers))
		}
		for _, item := range out.Providers {
			if item.ID == "openai-codex" && !item.SupportsLiveModels {
				t.Fatalf("expected openai-codex live_models=true, got %+v", item)
			}
		}
	}

func TestModelsAPI_OpenAICodexLiveModelsSupported_(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai-codex", "gpt-5.3-codex", "oauth", "https://chatgpt.com/backend-api")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{models: []string{"gpt-5.3-codex", "gpt-4.1-codex"}}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
		}
	if fetcher.calls != 1 {
		t.Fatalf("expected one fetch attempt, got %d", fetcher.calls)
	}
	if !strings.Contains(rec.Body.String(), `"provider":"openai-codex"`) {
		t.Fatalf("expected provider in response, got %q", rec.Body.String())
	}
	var out modelsAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if out.Provider != "openai-codex" {
		t.Fatalf("unexpected provider in response: %+v", out.Provider)
	}
	if len(out.Models) != 2 {
		t.Fatalf("expected 2 models, got %v", out.Models)
	}
}

func TestModelsAPI_UsesProviderAliasQueryParam_(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"default": {
					Kind:     "openai",
					AuthMode: "api-key",
					BaseURL:  "https://api.openai.com/v1",
				},
				"secondary": {
					Kind:     "anthropic",
					AuthMode: "api-key",
					BaseURL:  "https://api.anthropic.com",
				},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"standard": {Provider: "default", Model: "gpt-4o-mini"},
				"heavy":    {Provider: "secondary", Model: "claude-3-5-sonnet"},
			},
			LLMDefaultTier: "standard",
		},
	}
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{models: []string{"claude-3-opus", "claude-3-haiku"}}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models?provider_alias=secondary", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("expected one fetch attempt, got %d", fetcher.calls)
	}
	if fetcher.lastOps.Provider != "anthropic" {
		t.Fatalf("expected anthropic provider, got %+v", fetcher.lastOps.Provider)
	}
	if fetcher.lastOps.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("expected anthropic base URL, got %q", fetcher.lastOps.BaseURL)
	}

	var out modelsAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if out.Provider != "anthropic" {
		t.Fatalf("unexpected provider in response: %+v", out.Provider)
	}
	if out.CurrentModel != "claude-3-5-sonnet" {
		t.Fatalf("unexpected current model, got %q", out.CurrentModel)
	}
	if out.Source != "live" {
		t.Fatalf("expected live source, got %q", out.Source)
	}
	if len(out.Models) != 3 {
		t.Fatalf("expected 3 models including current model, got %+v", out.Models)
	}
}

func TestModelsAPI_UnknownProviderAlias_(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai", "gpt-4o-mini", "api-key", "https://api.openai.com/v1")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{models: []string{"gpt-4o-mini"}}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models?provider_alias=missing", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 0 {
		t.Fatalf("expected no fetch attempt, got %d", fetcher.calls)
	}
	if !strings.Contains(rec.Body.String(), "provider alias") {
		t.Fatalf("expected provider alias message, got %q", rec.Body.String())
	}
}

func TestModelsAPI_CacheHitFresh_(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai", "gpt-4o-mini", "api-key", "https://api.openai.com/v1")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	if err := cache.put("openai", "https://api.openai.com/v1", "api-key", []string{"gpt-4o-mini", "gpt-4.1"}, now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("cache.put: %v", err)
	}
	fetcher := &fakeModelFetcher{models: []string{"should-not-be-used"}}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 0 {
		t.Fatalf("expected no live fetch call, got %d", fetcher.calls)
	}

	var out modelsAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if out.Source != "cache" || out.Stale {
		t.Fatalf("expected fresh cache response, got %+v", out)
	}
	if len(out.Models) != 2 {
		t.Fatalf("expected 2 models, got %+v", out.Models)
	}
}

func TestModelsAPI_ExpiredLiveFailReturnsStale_(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai", "gpt-4o-mini", "api-key", "https://api.openai.com/v1")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	if err := cache.put("openai", "https://api.openai.com/v1", "api-key", []string{"gpt-4.1"}, now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("cache.put: %v", err)
	}
	fetcher := &fakeModelFetcher{err: fmt.Errorf("live provider unavailable")}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with stale cache, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("expected one live fetch attempt, got %d", fetcher.calls)
	}

	var out modelsAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if out.Source != "cache" || !out.Stale {
		t.Fatalf("expected stale cache response, got %+v", out)
	}
	if !strings.Contains(out.Warning, "live provider unavailable") {
		t.Fatalf("expected warning with live error, got %q", out.Warning)
	}
}

func TestModelsAPI_NoCacheLiveFail_(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai", "gpt-4o-mini", "api-key", "https://api.openai.com/v1")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{err: fmt.Errorf("provider outage")}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("expected one live fetch attempt, got %d", fetcher.calls)
	}
	if !strings.Contains(rec.Body.String(), "models_unavailable") {
		t.Fatalf("expected models_unavailable code, got %q", rec.Body.String())
	}
}

func TestModelsAPI_NoCachePermissionFail_(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("openai-codex", "gpt-5.3-codex", "oauth", "")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{
		err: &llm.ProviderError{
			Provider:   "openai-codex",
			Operation:  "request",
			StatusCode: http.StatusForbidden,
			Message:    `{"error":"You have insufficient permissions for this operation. Missing scopes: api.model.read."}`,
		},
	}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("expected one fetch attempt, got %d", fetcher.calls)
	}

	var out modelsAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if out.Provider != "openai-codex" {
		t.Fatalf("unexpected provider: %+v", out)
	}
	if out.Source != "provider_models_unavailable" {
		t.Fatalf("expected provider_models_unavailable source, got %q", out.Source)
	}
	if len(out.Models) != 0 {
		t.Fatalf("expected no models, got %+v", out.Models)
	}
	if !strings.Contains(out.Warning, "api.model.read") {
		t.Fatalf("expected warning, got %q", out.Warning)
	}
}

func TestProvidersAPI_ClaudeCodeCLIListedWithoutLiveModels(t *testing.T) {
	cfg := makePoolTestCfg("claude-code-cli", "sonnet", "cli", "")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, time.Now)
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(cfg, cache, &fakeModelFetcher{}, time.Now)
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var out providersAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode providers response: %v", err)
	}
	if out.CurrentProvider != "claude-code-cli" || out.CurrentModel != "sonnet" || out.AuthMode != "cli" {
		t.Fatalf("unexpected providers payload: %+v", out)
	}
	found := false
	for _, item := range out.Providers {
		if item.ID != "claude-code-cli" {
			continue
		}
		found = true
		if item.SupportsLiveModels {
			t.Fatalf("expected claude-code-cli live_models=false, got %+v", item)
		}
	}
	if !found {
		t.Fatalf("expected claude-code-cli in providers list, got %+v", out.Providers)
	}
}

func TestProvidersAPI_KimiListedWithLiveModels(t *testing.T) {
	cfg := makePoolTestCfg("kimi", "moonshot-v1-auto", "api-key", "https://api.moonshot.cn/v1")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, time.Now)
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(cfg, cache, &fakeModelFetcher{}, time.Now)
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var out providersAPIInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode providers response: %v", err)
	}
	if out.CurrentProvider != "kimi" || out.CurrentModel != "moonshot-v1-auto" || out.AuthMode != "api-key" {
		t.Fatalf("unexpected providers payload: %+v", out)
	}
	found := false
	for _, item := range out.Providers {
		if item.ID != "kimi" {
			continue
		}
		found = true
		if !item.SupportsLiveModels {
			t.Fatalf("expected kimi live_models=true, got %+v", item)
		}
	}
	if !found {
		t.Fatalf("expected kimi in providers list, got %+v", out.Providers)
	}
}

func TestModelsAPI_ClaudeCodeCLIUnsupported_(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	cfg := makePoolTestCfg("claude-code-cli", "sonnet", "cli", "")
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	fetcher := &fakeModelFetcher{models: []string{"should-not-be-used"}}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if fetcher.calls != 0 {
		t.Fatalf("expected no fetch attempt, got %d", fetcher.calls)
	}
	if !strings.Contains(rec.Body.String(), "claude-code-cli") {
		t.Fatalf("expected claude-code-cli message, got %q", rec.Body.String())
	}
}
