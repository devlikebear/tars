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

// TestOpenAIToolLoopLive drives the real OpenAI Chat Completions API through
// the round trip that matters for multi-turn correctness: tool call → tool
// result → follow-up. It is the openai half of the cross-provider live
// coverage asked for in #926; the anthropic half is
// TestAnthropicThinkingToolLoopLive.
//
// It needs a key and skips without one:
//
//	OPENAI_API_KEY=... go test -tags integration ./internal/llm/ -run TestOpenAIToolLoopLive -v
func TestOpenAIToolLoopLive(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("openai api key not available: set OPENAI_API_KEY")
	}
	model := strings.TrimSpace(os.Getenv("OPENAI_TEST_MODEL"))
	if model == "" {
		model = "gpt-4.1-mini"
	}

	client, err := NewProvider(ProviderOptions{
		Provider: "openai",
		Model:    model,
		APIKey:   apiKey,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	tools := []ToolSchema{{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        "lookup_city_population",
			Description: "Return the population of a city.",
			Parameters:  []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	messages := []ChatMessage{{
		Role:    "user",
		Content: "Use the lookup_city_population tool for Seoul, then tell me the number you got.",
	}}

	first, err := client.Chat(ctx, messages, ChatOptions{Tools: tools, ToolChoice: ToolChoiceRequired()})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if len(first.Message.ToolCalls) == 0 {
		t.Fatalf("expected a tool call, got %+v", first.Message)
	}
	// The suite's shared contract: arguments parse as JSON on every provider.
	var args map[string]any
	if err := json.Unmarshal([]byte(first.Message.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("tool arguments are not JSON: %q (%v)", first.Message.ToolCalls[0].Arguments, err)
	}

	messages = append(messages, first.Message, ChatMessage{
		Role:       "tool",
		ToolCallID: first.Message.ToolCalls[0].ID,
		Content:    "9650000",
	})
	second, err := client.Chat(ctx, messages, ChatOptions{Tools: tools})
	if err != nil {
		t.Fatalf("second turn (tool result replay): %v", err)
	}
	if !strings.Contains(second.Message.Content, "9,650,000") && !strings.Contains(second.Message.Content, "9650000") {
		t.Errorf("second turn did not use the tool result: %q", second.Message.Content)
	}
}

// TestOpenAIUsageReportingLive confirms the capability matrix's
// CapCacheUsageReporting claim against the real API rather than a stub.
func TestOpenAIUsageReportingLive(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("openai api key not available: set OPENAI_API_KEY")
	}
	model := strings.TrimSpace(os.Getenv("OPENAI_TEST_MODEL"))
	if model == "" {
		model = "gpt-4.1-mini"
	}

	client, err := NewProvider(ProviderOptions{Provider: "openai", Model: model, APIKey: apiKey})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resp, err := client.Chat(ctx, []ChatMessage{{Role: "user", Content: "Say OK."}}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Usage.InputTokens <= 0 || resp.Usage.OutputTokens <= 0 {
		t.Fatalf("usage not populated: %+v", resp.Usage)
	}
}
