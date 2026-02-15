package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		Model:    "gpt-4o-mini",
	})
	if err == nil {
		t.Fatal("expected error for removed codex-cli provider")
	}
}

func TestNewProvider_OpenAICodexIsRemoved(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "openai-codex",
		Model:    "gpt-5.3-codex",
	})
	if err == nil {
		t.Fatal("expected error for removed openai-codex provider")
	}
	if !strings.Contains(err.Error(), "removed") {
		t.Fatalf("expected removed error, got %v", err)
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
