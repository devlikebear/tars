package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	if cfg.HTTPTimeout != defaultHTTPTimeout {
		t.Fatalf("expected HTTPTimeout %v, got %v", defaultHTTPTimeout, cfg.HTTPTimeout)
	}
	if cfg.MaxTokens != 0 {
		t.Fatalf("expected MaxTokens 0, got %d", cfg.MaxTokens)
	}
}

func TestRequireConfiguredValue(t *testing.T) {
	t.Parallel()

	got, err := requireConfiguredValue("openai", "model", "  gpt-4o-mini  ")
	if err != nil {
		t.Fatalf("require configured value: %v", err)
	}
	if got != "gpt-4o-mini" {
		t.Fatalf("expected trimmed value, got %q", got)
	}

	_, err = requireConfiguredValue("anthropic", "api key", " \t ")
	if err == nil {
		t.Fatal("expected error for blank configured value")
	}
	if !strings.Contains(err.Error(), "anthropic api key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewOpenAICompatibleClientWithConfig_UsesConfig(t *testing.T) {
	cfg := ClientConfig{HTTPTimeout: 5 * time.Second, MaxTokens: 1234}

	client, err := newOpenAICompatibleClientWithConfig("openai", "http://localhost", "k", "m", cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config != cfg {
		t.Fatalf("expected config %+v, got %+v", cfg, client.config)
	}
	if client.httpClient.Timeout != cfg.HTTPTimeout {
		t.Fatalf("expected timeout %v, got %v", cfg.HTTPTimeout, client.httpClient.Timeout)
	}
}

func TestNewAnthropicClientWithConfig_UsesMaxTokensFromConfig(t *testing.T) {
	cfg := ClientConfig{HTTPTimeout: 3 * time.Second, MaxTokens: 777}

	var captured struct {
		MaxTokens int `json:"max_tokens"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	client, err := newAnthropicClientWithConfig(srv.URL, "k", "claude-3-5-haiku-latest", cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config != cfg {
		t.Fatalf("expected config %+v, got %+v", cfg, client.config)
	}
	if client.httpClient.Timeout != cfg.HTTPTimeout {
		t.Fatalf("expected timeout %v, got %v", cfg.HTTPTimeout, client.httpClient.Timeout)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if captured.MaxTokens != cfg.MaxTokens {
		t.Fatalf("expected max_tokens %d, got %d", cfg.MaxTokens, captured.MaxTokens)
	}
}

func TestNewAnthropicClient_DefaultsMaxTokensTo4096(t *testing.T) {
	client, err := NewAnthropicClient("http://localhost", "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config.MaxTokens != 4096 {
		t.Fatalf("expected MaxTokens 4096, got %d", client.config.MaxTokens)
	}
}

func TestNewGeminiNativeClientWithConfig_UsesConfig(t *testing.T) {
	cfg := ClientConfig{HTTPTimeout: 7 * time.Second, MaxTokens: 2048}

	client, err := newGeminiNativeClientWithConfig("http://localhost/v1beta", "k", "gemini-2.5-flash", cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config != cfg {
		t.Fatalf("expected config %+v, got %+v", cfg, client.config)
	}
	if client.httpClient.Timeout != cfg.HTTPTimeout {
		t.Fatalf("expected timeout %v, got %v", cfg.HTTPTimeout, client.httpClient.Timeout)
	}
}
