package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
	"github.com/devlikebear/tars/internal/config"
)

func TestSetupDiscoveryReadyProviders(t *testing.T) {
	runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
		"--version":          {out: []byte("2.1.284")},
		"auth status --json": {out: []byte(`{"loggedIn":true,"authMethod":"oauth"}`)},
	}}
	service := newProbeTestService(t, config.Config{}, &fakeModelFetcher{}, runner)
	discovery := newSetupDiscovery(service)
	discovery.findCodex = func() (string, error) { return "/bin/codex", nil }
	discovery.codexCredential = func() (auth.ProviderCredential, error) {
		return auth.ProviderCredential{AccessToken: "private-token"}, nil
	}
	discovery.codexModels = func(context.Context) (modelsAPIInfo, error) {
		return modelsAPIInfo{Source: "live", Models: []string{"new-default", "new-fast"}}, nil
	}
	got := discovery.discover(context.Background())
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates: %+v", got)
	}
	claude, codex := got.Candidates[0], got.Candidates[1]
	if !claude.Ready || claude.Recommended["standard"] != "sonnet" || claude.Recommended["heavy"] != "opus" || claude.Recommended["light"] != "haiku" {
		t.Fatalf("Claude: %+v", claude)
	}
	if !codex.Ready || codex.Recommended["standard"] != "new-default" || codex.Source != "live" {
		t.Fatalf("Codex: %+v", codex)
	}
	if len(service.cfg.LLMProviders) != 0 {
		t.Fatal("discovery changed saved providers")
	}
}

func TestSetupDiscoveryMissingAndUnusableAuth(t *testing.T) {
	service := newProbeTestService(t, config.Config{}, &fakeModelFetcher{}, &fakeCLIRunner{})
	service.findCLI = func(string) (string, error) { return "", errors.New("missing") }
	discovery := newSetupDiscovery(service)
	discovery.findCodex = func() (string, error) { return "/bin/codex", nil }
	discovery.codexCredential = func() (auth.ProviderCredential, error) {
		return auth.ProviderCredential{}, errors.New("secret should never be returned")
	}
	discovery.codexModels = func(context.Context) (modelsAPIInfo, error) {
		t.Fatal("models requested without auth")
		return modelsAPIInfo{}, nil
	}
	got := discovery.discover(context.Background())
	if got.Candidates[0].Installed || got.Candidates[0].Ready {
		t.Fatalf("missing Claude: %+v", got)
	}
	if !got.Candidates[1].Installed || got.Candidates[1].Ready || got.Candidates[1].Problem != "not_logged_in" {
		t.Fatalf("unusable Codex: %+v", got)
	}
}

func TestSetupDiscoveryCachedModels(t *testing.T) {
	service := newProbeTestService(t, config.Config{}, &fakeModelFetcher{}, &fakeCLIRunner{})
	service.findCLI = func(string) (string, error) { return "", errors.New("missing") }
	discovery := newSetupDiscovery(service)
	discovery.findCodex = func() (string, error) { return "/bin/codex", nil }
	discovery.codexCredential = func() (auth.ProviderCredential, error) {
		return auth.ProviderCredential{AccessToken: "private-token"}, nil
	}
	discovery.codexModels = func(context.Context) (modelsAPIInfo, error) {
		return modelsAPIInfo{Source: "cache", Stale: true, Models: []string{"cached-model"}}, nil
	}
	got := discovery.discover(context.Background()).Candidates[1]
	if !got.Ready || got.Source != "cache" || got.Recommended["standard"] != "cached-model" {
		t.Fatalf("cached recommendation: %+v", got)
	}
}

func TestSetupDiscoveryRejectsExpiredAuthentication(t *testing.T) {
	service := newProbeTestService(t, config.Config{}, &fakeModelFetcher{}, &fakeCLIRunner{})
	service.findCLI = func(string) (string, error) { return "", errors.New("missing") }
	d := newSetupDiscovery(service)
	d.findCodex = func() (string, error) { return "/bin/codex", nil }
	d.codexCredential = func() (auth.ProviderCredential, error) { return auth.ProviderCredential{AccessToken: "expired"}, nil }
	d.codexModels = func(context.Context) (modelsAPIInfo, error) {
		return modelsAPIInfo{Source: "fallback", Models: []string{"old-model"}}, &llm.ProviderError{StatusCode: http.StatusUnauthorized}
	}
	candidate := d.discover(context.Background()).Candidates[1]
	if candidate.Ready || candidate.Problem != providerProbeNotLoggedIn {
		t.Fatalf("expired login should not auto-configure: %+v", candidate)
	}
}
func TestSetupDiscoveryModelsRefreshAndFallback(t *testing.T) {
	fetcher := &fakeModelFetcher{models: []string{"first-recommended", "next-model"}}
	service := newProbeTestService(t, config.Config{}, fetcher, &fakeCLIRunner{})
	d := newSetupDiscovery(service)
	info, err := d.models(context.Background())
	if err != nil || info.Source != "live" || info.Models[0] != "first-recommended" {
		t.Fatalf("live: %+v %v", info, err)
	}
	fetcher.err = errors.New("offline")
	info, err = d.models(context.Background())
	if err != nil || info.Source != "cache" || info.Models[0] != "first-recommended" {
		t.Fatalf("cache: %+v %v", info, err)
	}
	service.cache = nil
	info, err = d.models(context.Background())
	if err == nil || info.Source != "fallback" || len(info.Models) == 0 {
		t.Fatalf("fallback: %+v %v", info, err)
	}
}
func TestSetupDiscoveryRecommendationRoles(t *testing.T) {
	got := recommendCodexModels([]string{"latest-astra", "latest-sol", "latest-luna"})
	if got["heavy"] != "latest-astra" || got["standard"] != "latest-sol" || got["light"] != "latest-luna" {
		t.Fatalf("roles: %+v", got)
	}
	if len(recommendCodexModels(nil)) != 0 {
		t.Fatal("empty catalog should not invent a model")
	}
}
func TestSetupDiscoveryNeverReturnsCredentials(t *testing.T) {
	service := newProbeTestService(t, config.Config{}, &fakeModelFetcher{}, &fakeCLIRunner{})
	service.findCLI = func(string) (string, error) { return "", errors.New("missing") }
	d := newSetupDiscovery(service)
	d.findCodex = func() (string, error) { return "", errors.New("missing") }
	d.codexCredential = func() (auth.ProviderCredential, error) { return auth.ProviderCredential{}, errors.New("secret-detail") }
	data, err := json.Marshal(d.discover(context.Background()))
	if err != nil || strings.Contains(string(data), "secret-detail") {
		t.Fatalf("unexpected response: %s %v", data, err)
	}
}
func TestFindLocalCodex(t *testing.T) {
	dir := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name = "codex.exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := findLocalCodex()
	if err != nil || got != path {
		t.Fatalf("lookup: %s %v", got, err)
	}
}
func TestSetupDiscoveryHandlerRequiresGET(t *testing.T) {
	handler := newProvidersModelsAPIHandler(nil, zerolog.Nop())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/setup/discover", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/setup/discover", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable: %d", rec.Code)
	}
}
