//go:build integration

package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGeminiLive_OpenAICompatibleToolCall(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is not set")
	}

	model := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if model == "" {
		model = "gemini-3.8-flash"
	}
	baseURL := strings.TrimSpace(os.Getenv("GEMINI_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}

	client, err := NewProvider(ProviderOptions{
		Provider: "gemini",
		AuthMode: "api-key",
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Call the echo_tool function with {\"text\":\"ok\"}."},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "echo_tool",
					Description: "echo text",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
				},
			},
		},
		ToolChoice: ToolChoiceRequired(),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) == 0 {
		t.Fatalf("expected tool call, got %+v", resp)
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "echo_tool" {
		t.Fatalf("unexpected tool call name: %q", call.Name)
	}
	if !json.Valid([]byte(call.Arguments)) {
		t.Fatalf("tool arguments should be valid JSON: %q", call.Arguments)
	}
}

func TestGeminiLive_NativeToolCall(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is not set")
	}

	model := strings.TrimSpace(os.Getenv("GEMINI_NATIVE_MODEL"))
	if model == "" {
		model = "gemini-3.8-flash"
	}
	baseURL := strings.TrimSpace(os.Getenv("GEMINI_NATIVE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}

	client, err := NewProvider(ProviderOptions{
		Provider: "gemini-native",
		AuthMode: "api-key",
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Call the echo_tool function with {\"text\":\"ok\"}."},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "echo_tool",
					Description: "echo text",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
				},
			},
		},
		ToolChoice: ToolChoiceRequired(),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) == 0 {
		t.Fatalf("expected tool call, got %+v", resp)
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "echo_tool" {
		t.Fatalf("unexpected tool call name: %q", call.Name)
	}
	if !json.Valid([]byte(call.Arguments)) {
		t.Fatalf("tool arguments should be valid JSON: %q", call.Arguments)
	}
}

// TestGeminiLive_EffortAndImplicitCache sends reasoning_effort to the native
// API, which rejected the THINKING_LEVEL_ spelling with a 400, and repeats a
// long prompt until implicit caching reports a hit on each Gemini kind.
func TestGeminiLive_EffortAndImplicitCache(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is not set")
	}
	model := strings.TrimSpace(os.Getenv("GEMINI_NATIVE_MODEL"))
	if model == "" {
		model = "gemini-3.8-flash"
	}
	long := strings.Repeat("TARS implicit cache probe: a sentence from a long design document. ", 500) + "\nReply with the single word: ok"

	for _, kind := range []struct{ provider, baseURL string }{
		{"gemini-native", "https://generativelanguage.googleapis.com/v1beta"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
	} {
		t.Run(kind.provider, func(t *testing.T) {
			opts := ProviderOptions{Provider: kind.provider, AuthMode: "api-key", BaseURL: kind.baseURL, APIKey: apiKey, Model: model}
			if kind.provider == "gemini-native" {
				opts.ReasoningEffort = "low"
			}
			client, err := NewProvider(opts)
			if err != nil {
				t.Fatalf("new provider: %v", err)
			}
			for attempt := 1; attempt <= 4; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				resp, err := client.Chat(ctx, []ChatMessage{{Role: "user", Content: long}}, ChatOptions{})
				cancel()
				if err != nil {
					t.Fatalf("attempt %d: %v", attempt, err)
				}
				t.Logf("attempt %d usage: %+v", attempt, resp.Usage)
				if resp.Usage.CachedTokens > 0 {
					return
				}
			}
			t.Fatalf("no implicit cache hit reported in 4 identical requests")
		})
	}
}
