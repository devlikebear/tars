package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/devlikebear/tarsncase/internal/auth"
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
		resolveToken: func(opts auth.ResolveOptions) (string, error) {
			if opts.Provider != "openai" {
				t.Fatalf("expected openai provider, got %q", opts.Provider)
			}
			return "openai-token", nil
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
		resolveToken: func(opts auth.ResolveOptions) (string, error) {
			if opts.Provider != "anthropic" {
				t.Fatalf("expected anthropic provider, got %q", opts.Provider)
			}
			return "anthropic-token", nil
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
		resolveToken: func(opts auth.ResolveOptions) (string, error) {
			if opts.Provider != "gemini" {
				t.Fatalf("expected gemini provider, got %q", opts.Provider)
			}
			return "gemini-key", nil
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

func TestModelFetcher_OpenAICodexRefreshRetry401_(t *testing.T) {
	t.Helper()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		switch got := r.Header.Get("Authorization"); got {
		case "Bearer old-token":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"expired"}`)
		case "Bearer new-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-5.3-codex"},
					{"id": "gpt-4.1-codex"},
				},
			})
		default:
			t.Fatalf("unexpected authorization header: %q", got)
		}
	}))
	defer server.Close()

	refreshCalls := 0
	fetcher := newModelFetcherWithDeps(modelFetcherDeps{
		httpClient:           server.Client(),
		openAICodexModelsURL: server.URL + "/v1/models",
		resolveCodexCredential: func(opts auth.CodexResolveOptions) (auth.CodexCredential, error) {
			return auth.CodexCredential{
				AccessToken:  "old-token",
				RefreshToken: "refresh-token",
				Source:       auth.CodexCredentialSourceFile,
				SourcePath:   "/tmp/auth.json",
			}, nil
		},
		refreshCodexCredential: func(ctx context.Context, cred auth.CodexCredential, opts auth.CodexRefreshOptions) (auth.CodexCredential, error) {
			refreshCalls++
			if !opts.PersistFile {
				t.Fatalf("expected PersistFile=true")
			}
			if cred.RefreshToken != "refresh-token" {
				t.Fatalf("expected refresh token refresh-token, got %q", cred.RefreshToken)
			}
			return auth.CodexCredential{
				AccessToken:  "new-token",
				RefreshToken: "refresh-token-2",
				Source:       auth.CodexCredentialSourceFile,
				SourcePath:   "/tmp/auth.json",
			}, nil
		},
	})

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{
		Provider: "openai-codex",
	})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if refreshCalls != 1 {
		t.Fatalf("expected refresh call once, got %d", refreshCalls)
	}
	if requests != 2 {
		t.Fatalf("expected two requests (retry after refresh), got %d", requests)
	}
	want := []string{"gpt-4.1-codex", "gpt-5.3-codex"}
	if !slices.Equal(models, want) {
		t.Fatalf("unexpected models: want=%v got=%v", want, models)
	}
}
