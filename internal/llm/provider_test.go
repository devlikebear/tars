package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewProvider_Unsupported(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "unknown",
		AuthMode: "api-key",
		APIKey:   "k",
	})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewProvider_CodexCLIIsRemoved(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "codex-cli",
		Model:    "gpt-5-codex",
	})
	if err == nil {
		t.Fatal("expected error for removed codex-cli provider")
	}
}

func TestNewProvider_OpenAICodexRequiresExperimentalFlag(t *testing.T) {
	t.Setenv("CODEX_OAUTH_TOKEN", "token")

	_, err := NewProvider(ProviderOptions{
		Provider: "openai-codex",
		AuthMode: "oauth",
		Model:    "gpt-5-codex",
	})
	if err == nil {
		t.Fatal("expected error without experimental flag")
	}
}

func TestNewProvider_OpenAICodexFallsBackToOpenAI(t *testing.T) {
	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"blocked"}`))
	}))
	defer primary.Close()

	fallbackCalls := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fallback response"}}]}`))
	}))
	defer fallback.Close()

	t.Setenv("CODEX_OAUTH_TOKEN", "codex-token")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", fallback.URL+"/v1")

	client, err := NewProvider(ProviderOptions{
		Provider:          "openai-codex",
		AuthMode:          "oauth",
		AllowExperimental: true,
		BaseURL:           primary.URL + "/v1",
		Model:             "gpt-5-codex",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hello"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "fallback response" {
		t.Fatalf("unexpected response: %q", resp.Message.Content)
	}
	if primaryCalls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primaryCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback to be called once, got %d", fallbackCalls)
	}
}
