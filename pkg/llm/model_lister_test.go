package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
)

func TestModelFetcher_OpenAICompatible_Parses(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer openai-token" {
			t.Fatalf("expected Bearer token header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o-mini"},
				{"id": "gpt-4o-mini"},
				{"id": "gpt-4.1"},
			},
		})
	}))
	defer server.Close()

	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient: server.Client(),
		resolveCredential: func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			if config.Provider != "openai" {
				t.Fatalf("expected openai provider, got %+v", config)
			}
			return auth.ProviderCredential{AccessToken: "openai-token"}, nil
		},
	})

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{
		Provider: "openai",
		BaseURL:  server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	want := []string{"gpt-4.1", "gpt-4o-mini"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}

func TestModelFetcher_Anthropic_Parses(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "anthropic-token" {
			t.Fatalf("expected x-api-key header, got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicAPIVersion {
			t.Fatalf("expected anthropic-version %q, got %q", anthropicAPIVersion, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-4-6"},
				{"id": "claude-haiku-4-5"},
			},
		})
	}))
	defer server.Close()

	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient: server.Client(),
		resolveCredential: func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			if config.Provider != "anthropic" {
				t.Fatalf("expected anthropic provider, got %+v", config)
			}
			return auth.ProviderCredential{AccessToken: "anthropic-token"}, nil
		},
	})

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{
		Provider: "anthropic",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	want := []string{"claude-haiku-4-5", "claude-sonnet-4-6"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}

func TestModelFetcher_Kimi_UsesOpenAIStylePathAndAuth(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer kimi-token" {
			t.Fatalf("expected Bearer token header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "moonshot-v1-auto"},
				{"id": "moonshot-v1-8k"},
			},
		})
	}))
	defer server.Close()

	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient: server.Client(),
		resolveCredential: func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			if config.Provider != "kimi" {
				t.Fatalf("expected kimi provider, got %+v", config)
			}
			return auth.ProviderCredential{AccessToken: "kimi-token"}, nil
		},
	})

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{
		Provider: "kimi",
		BaseURL:  server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	want := []string{"moonshot-v1-8k", "moonshot-v1-auto"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}

func TestModelFetcher_GeminiNativeSinglePath_Parses(t *testing.T) {
	t.Helper()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/v1beta/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Fatalf("expected x-goog-api-key header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "models/gemini-2.5-flash"},
				{"name": "models/gemini-2.0-pro"},
			},
		})
	}))
	defer server.Close()

	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient: server.Client(),
		resolveCredential: func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			if config.Provider != "gemini" {
				t.Fatalf("expected gemini provider, got %+v", config)
			}
			return auth.ProviderCredential{AccessToken: "gemini-key"}, nil
		},
	})

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{
		Provider: "gemini",
		BaseURL:  server.URL + "/v1beta/openai",
	})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected single request, got %d", requests)
	}
	want := []string{"gemini-2.0-pro", "gemini-2.5-flash"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}

// The Codex backend's real response shape, trimmed to the fields that matter
// (codex-rs/protocol/src/openai_models.rs). Priorities are deliberately out
// of slug order so the test can tell "sorted by priority" from "sorted by
// name"; the hidden entry is what codex-auto-review looks like in the wild.
const codexModelsResponse = `{
  "fetched_at": "2026-09-08T08:17:39Z",
  "models": [
    {"slug": "gpt-5.6-sol",  "display_name": "GPT-5.6-Sol", "visibility": "list", "supported_in_api": true,  "priority": 6},
    {"slug": "gpt-6-astra",  "display_name": "GPT-6-Astra", "visibility": "list", "supported_in_api": true,  "priority": 1},
    {"slug": "codex-auto-review", "display_name": "Codex Auto Review", "visibility": "hide", "supported_in_api": true, "priority": 43},
    {"slug": "gpt-5.4-mini", "display_name": "GPT-5.4-Mini", "visibility": "list", "supported_in_api": true, "priority": 23}
  ]
}`

func codexFetcherFor(t *testing.T, server *httptest.Server, refresh func(auth.ProviderCredential) (auth.ProviderCredential, error)) (*modelFetcher, *int) {
	t.Helper()
	refreshCalls := 0
	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient:           server.Client(),
		openAICodexModelsURL: server.URL + "/backend-api/codex/models",
		resolveCredential: func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			if config.Provider != "openai-codex" || config.AuthMode != "oauth" {
				t.Fatalf("unexpected auth config: %+v", config)
			}
			return auth.ProviderCredential{
				AccessToken:  "old-token",
				RefreshToken: "refresh-token",
				AccountID:    "acct-42",
				Source:       auth.CredentialSourceFile,
				SourcePath:   "/tmp/auth.json",
			}, nil
		},
		refreshCredential: func(_ context.Context, config auth.ProviderAuthConfig, cred auth.ProviderCredential, opts auth.ProviderRefreshOptions) (auth.ProviderCredential, error) {
			refreshCalls++
			if config.Provider != "openai-codex" || config.AuthMode != "oauth" {
				t.Fatalf("unexpected auth config: %+v", config)
			}
			if !opts.PersistSource {
				t.Fatalf("expected PersistSource=true")
			}
			if refresh == nil {
				t.Fatal("refresh was not expected on this path")
			}
			return refresh(cred)
		},
	})
	return fetcher, &refreshCalls
}

// The bug this pins: the list used to be fetched from api.openai.com, the
// OpenAI platform API, which does not accept a ChatGPT OAuth token and
// answered 401 every time. It must go to the ChatGPT backend's own
// /codex/models, with the same identity headers the chat request sends and
// the client_version query the CLI sends, and parse the backend's
// {models:[{slug,...}]} shape rather than OpenAI's {data:[{id}]}.
func TestModelFetcher_OpenAICodex_ListsFromTheChatGPTBackend(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/backend-api/codex/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("client_version"); got == "" {
			t.Fatalf("expected a client_version query, got url %q", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer old-token" {
			t.Fatalf("expected the bearer token, got %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acct-42" {
			t.Fatalf("expected chatgpt-account-id acct-42, got %q", got)
		}
		if got := r.Header.Get("originator"); got == "" {
			t.Fatal("expected an originator header, as the chat request sends")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, codexModelsResponse)
	}))
	defer server.Close()

	fetcher, refreshCalls := codexFetcherFor(t, server, nil)
	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{Provider: "openai-codex"})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one request, got %d", requests)
	}
	if *refreshCalls != 0 {
		t.Fatalf("a 200 must not spend a refresh, got %d", *refreshCalls)
	}
	// Priority order, not name order; the hidden model is not offered.
	want := []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.4-mini"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}

// A 401 from the RIGHT host is a genuinely stale token: refresh once, retry
// once, at the same URL. This is the retry the old code performed against the
// wrong host, where it could never succeed.
func TestModelFetcher_OpenAICodex_RefreshesOnceOn401ThenRetries(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/backend-api/codex/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch got := r.Header.Get("Authorization"); got {
		case "Bearer old-token":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"expired"}`)
		case "Bearer new-token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, codexModelsResponse)
		default:
			t.Fatalf("unexpected authorization header: %q", got)
		}
	}))
	defer server.Close()

	fetcher, refreshCalls := codexFetcherFor(t, server, func(cred auth.ProviderCredential) (auth.ProviderCredential, error) {
		if cred.RefreshToken != "refresh-token" {
			t.Fatalf("expected refresh token refresh-token, got %q", cred.RefreshToken)
		}
		return auth.ProviderCredential{
			AccessToken:  "new-token",
			RefreshToken: "refresh-token-2",
			AccountID:    "acct-42",
			Source:       auth.CredentialSourceFile,
			SourcePath:   "/tmp/auth.json",
		}, nil
	})
	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{Provider: "openai-codex"})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if *refreshCalls != 1 {
		t.Fatalf("expected refresh once, got %d", *refreshCalls)
	}
	if requests != 2 {
		t.Fatalf("expected two requests (retry after refresh), got %d", requests)
	}
	if !slices.Equal(models, []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.4-mini"}) {
		t.Fatalf("unexpected models after refresh: %v", models)
	}
}

// A 400/500 is not an auth problem and must not spend a refresh token.
func TestModelFetcher_OpenAICodex_NonAuthErrorDoesNotRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer server.Close()

	fetcher, refreshCalls := codexFetcherFor(t, server, nil)
	if _, err := fetcher.FetchModels(context.Background(), ProviderOptions{Provider: "openai-codex"}); err == nil {
		t.Fatal("expected an error")
	}
	if *refreshCalls != 0 {
		t.Fatalf("a 500 must not spend a refresh, got %d", *refreshCalls)
	}
}

func TestResolveOpenAICodexModelsURL(t *testing.T) {
	cases := map[string]string{
		"":                                      "https://chatgpt.com/backend-api/codex/models",
		"https://chatgpt.com/backend-api":       "https://chatgpt.com/backend-api/codex/models",
		"https://chatgpt.com/backend-api/":      "https://chatgpt.com/backend-api/codex/models",
		"https://chatgpt.com/backend-api/codex": "https://chatgpt.com/backend-api/codex/models",
		"https://chatgpt.com/backend-api/codex/models": "https://chatgpt.com/backend-api/codex/models",
		"https://proxy.example/api":                    "https://proxy.example/api/codex/models",
	}
	for in, want := range cases {
		if got := resolveOpenAICodexModelsURL(in); got != want {
			t.Errorf("resolveOpenAICodexModelsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOpenAICodexModelSlugs_EmptyListIsAnError(t *testing.T) {
	if _, err := parseOpenAICodexModelSlugs([]byte(`{"models":[{"slug":"x","visibility":"hide","priority":1}]}`)); err == nil {
		t.Fatal("a response with no listable model must be an error, not an empty success")
	}
	if _, err := parseOpenAICodexModelSlugs([]byte(`{"data":[{"id":"gpt-4o"}]}`)); err == nil {
		t.Fatal("an OpenAI-platform-shaped body must not be mistaken for a Codex list")
	}
}
