package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
)

func TestNormalizeLowerTrimmed(t *testing.T) {
	t.Parallel()

	if got := normalizeLowerTrimmed("  OpenAI-Codex "); got != "openai-codex" {
		t.Fatalf("expected openai-codex, got %q", got)
	}
	if got := normalizeLowerTrimmed(" \t "); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

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
		Model:    "gpt-4o-mini",
	})
	if err == nil {
		t.Fatal("expected error for removed codex-cli provider")
	}
}

func TestNewProvider_OpenAICodex_UsesCodexClient(t *testing.T) {
	client, err := NewProvider(ProviderOptions{
		Provider: "openai-codex",
		AuthMode: "api-key",
		APIKey:   "token",
		BaseURL:  "https://chatgpt.com/backend-api",
		Model:    "gpt-5.3-codex",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := client.(*OpenAICodexClient); !ok {
		t.Fatalf("expected OpenAICodexClient, got %T", client)
	}
}

func TestProviderOptionsAuthConfig_DefaultsOpenAICodexOAuth(t *testing.T) {
	config := providerAuthConfig(ProviderOptions{
		Provider: "openai-codex",
		APIKey:   "token",
	})
	if config.Provider != "openai-codex" {
		t.Fatalf("expected provider openai-codex, got %+v", config)
	}
	if config.AuthMode != "oauth" {
		t.Fatalf("expected default auth mode oauth, got %+v", config)
	}
	if config.OAuthProvider != "openai-codex" {
		t.Fatalf("expected default oauth provider openai-codex, got %+v", config)
	}
	if config.APIKey != "token" {
		t.Fatalf("expected api key to carry through, got %+v", config)
	}
}

func TestProviderOptionsAuthConfig_PrefersExplicitAuthConfig(t *testing.T) {
	config := providerAuthConfig(ProviderOptions{
		Provider:      "openai",
		AuthMode:      "api-key",
		OAuthProvider: "legacy",
		APIKey:        "legacy-key",
		AuthConfig: auth.ProviderAuthConfig{
			Provider:      "anthropic",
			AuthMode:      "oauth",
			OAuthProvider: "claude-code",
			APIKey:        "override-key",
		},
	})
	if config.Provider != "anthropic" || config.AuthMode != "oauth" || config.OAuthProvider != "claude-code" {
		t.Fatalf("expected explicit auth config to win, got %+v", config)
	}
	if config.APIKey != "override-key" {
		t.Fatalf("expected explicit api key override, got %+v", config)
	}
}

func TestNewProvider_OpenAICodex_PassesExplicitAuthConfig(t *testing.T) {
	client, err := NewProvider(ProviderOptions{
		Provider: "openai-codex",
		BaseURL:  "https://chatgpt.com/backend-api",
		Model:    "gpt-5.3-codex",
		AuthConfig: auth.ProviderAuthConfig{
			Provider:  "openai-codex",
			AuthMode:  "api-key",
			APIKey:    "token",
			CodexHome: "/tmp/custom-codex-home",
		},
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	codexClient, ok := client.(*OpenAICodexClient)
	if !ok {
		t.Fatalf("expected OpenAICodexClient, got %T", client)
	}
	if codexClient.authConfig.AuthMode != "api-key" || codexClient.authConfig.APIKey != "token" {
		t.Fatalf("unexpected auth config: %+v", codexClient.authConfig)
	}
	if codexClient.authConfig.CodexHome != "/tmp/custom-codex-home" {
		t.Fatalf("expected CodexHome override to propagate, got %+v", codexClient.authConfig)
	}
}

func TestNewProvider_GeminiToolCall(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"content":"",
					"tool_calls":[{"id":"call_1","function":{"name":"memory_search","arguments":"{\"query\":\"coffee\"}"}}]
				},
				"finish_reason":"tool_calls"
			}]
		}`))
	}))
	defer srv.Close()

	client, err := NewProvider(ProviderOptions{
		Provider: "gemini",
		AuthMode: "api-key",
		BaseURL:  srv.URL + "/v1beta/openai",
		APIKey:   "gemini-key",
		Model:    "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "find coffee memory"},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "memory_search",
					Description: "search memory",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
				},
			},
		},
		ToolChoice: "required",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if captured["tool_choice"] != "required" {
		t.Fatalf("expected tool_choice required, got %+v", captured["tool_choice"])
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
}

func TestNewProvider_GeminiStreamingToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"exec\",\"arguments\":\"{\\\"command\\\":\\\"p\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"wd\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewProvider(ProviderOptions{
		Provider: "gemini",
		AuthMode: "api-key",
		BaseURL:  srv.URL + "/v1beta/openai",
		APIKey:   "gemini-key",
		Model:    "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "run pwd"},
	}, ChatOptions{
		OnDelta: func(string) {},
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:       "exec",
					Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
				},
			},
		},
		ToolChoice: "required",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Arguments != "{\"command\":\"pwd\"}" {
		t.Fatalf("unexpected tool args: %q", resp.Message.ToolCalls[0].Arguments)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("expected tool_calls stop reason, got %q", resp.StopReason)
	}
}

func TestNewProvider_GeminiNativeToolCall(t *testing.T) {
	var preflightCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1beta/models/gemini-2.5-pro" {
			preflightCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-2.5-pro","supportedGenerationMethods":["generateContent"]}`))
			return
		}
		if r.URL.Path != "/v1beta/models/gemini-2.5-pro:generateContent" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"functionCall":{"name":"memory_search","args":{"query":"coffee"}}}]},"finishReason":"STOP"}]
		}`))
	}))
	defer srv.Close()

	client, err := NewProvider(ProviderOptions{
		Provider: "gemini-native",
		AuthMode: "api-key",
		BaseURL:  srv.URL + "/v1beta",
		APIKey:   "gemini-key",
		Model:    "gemini-2.5-pro",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "find memory"},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:       "memory_search",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
		ToolChoice: "required",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
	if preflightCalls != 1 {
		t.Fatalf("expected one preflight call, got %d", preflightCalls)
	}
}
