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
		model = "gemini-2.5-flash"
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
		ToolChoice: "required",
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
		model = "gemini-2.5-flash"
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
		ToolChoice: "required",
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
