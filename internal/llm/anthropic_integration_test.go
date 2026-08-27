//go:build integration

package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAnthropicThinkingToolLoopLive drives the real Messages API through the
// exact shape LP-003 exists to fix: extended thinking enabled *and* tools
// present, across more than one loop iteration. Before that change the second
// iteration replayed the assistant turn without its signed thinking blocks and
// the API rejected it.
//
// It runs against one model per thinking generation, because the request shape
// differs between them and a single model proves only its own half. The
// original version of this test defaulted to claude-sonnet-4-5 alone — the
// last generation that takes budget_tokens — which is why it passed while
// every adaptive-thinking model was being sent a request they reject (#943).
//
// ANTHROPIC_TEST_MODELS overrides the list (comma-separated). It needs a key
// and skips without one:
//
//	ANTHROPIC_API_KEY=... go test -tags integration ./internal/llm/ -run TestAnthropicThinkingToolLoopLive -v
func TestAnthropicThinkingToolLoopLive(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		t.Skip("anthropic api key not available: set ANTHROPIC_API_KEY")
	}

	models := []string{
		"claude-sonnet-4-5", // budget generation: thinking.budget_tokens
		"claude-opus-4-7",   // adaptive generation: output_config.effort
	}
	if override := strings.TrimSpace(os.Getenv("ANTHROPIC_TEST_MODELS")); override != "" {
		models = nil
		for _, entry := range strings.Split(override, ",") {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				models = append(models, trimmed)
			}
		}
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			runAnthropicThinkingToolLoop(t, apiKey, model)
		})
	}
}

func runAnthropicThinkingToolLoop(t *testing.T, apiKey, model string) {
	t.Helper()

	client, err := NewProvider(ProviderOptions{
		Provider:  "anthropic",
		Model:     model,
		APIKey:    apiKey,
		BaseURL:   "https://api.anthropic.com",
		MaxTokens: 8192,
		// The whole point: an effort level, not a hand-written budget.
		ReasoningEffort: "low",
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
	if len(first.Message.ReasoningBlocks) == 0 {
		t.Fatalf("expected reasoning blocks on a thinking turn, got %+v", first.Message)
	}
	for _, block := range first.Message.ReasoningBlocks {
		if block.Type == ReasoningBlockThinking && strings.TrimSpace(block.Signature) == "" {
			t.Fatalf("thinking block came back unsigned: %+v", block)
		}
	}

	// Replay the assistant turn plus a tool result. This is the request that
	// used to fail.
	messages = append(messages, first.Message, ChatMessage{
		Role:       "tool",
		ToolCallID: first.Message.ToolCalls[0].ID,
		Content:    "9650000",
	})
	second, err := client.Chat(ctx, messages, ChatOptions{Tools: tools})
	if err != nil {
		t.Fatalf("second turn (thinking block replay): %v", err)
	}
	if !strings.Contains(second.Message.Content, "9,650,000") && !strings.Contains(second.Message.Content, "9650000") {
		t.Errorf("second turn did not use the tool result: %q", second.Message.Content)
	}
}
