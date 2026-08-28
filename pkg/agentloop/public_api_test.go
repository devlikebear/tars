package agentloop_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/tools"
)

type scriptedClient struct {
	calls int
}

func (c *scriptedClient) Ask(context.Context, string) (string, error) {
	return "", nil
}

func (c *scriptedClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		if len(opts.Tools) != 1 || opts.Tools[0].Function.Name != "echo" {
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "missing tool schema"}}, nil
		}
		return llm.ChatResponse{
			Message: llm.ChatMessage{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:        "call_echo",
					Name:      "echo",
					Arguments: `{"text":"hello from pkg"}`,
				}},
			},
		}, nil
	}
	last := messages[len(messages)-1]
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "tool said " + last.Content,
		},
	}, nil
}

func TestLoopRunsRegisteredToolThroughPublicPackages(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.Tool{
		Name:        "echo",
		Description: "Echo text back to the model.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Execute: func(_ context.Context, params json.RawMessage) (tools.Result, error) {
			var input struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return tools.JSONTextResult(map[string]string{"error": err.Error()}, true), nil
			}
			return tools.JSONTextResult(map[string]string{"echo": input.Text}, false), nil
		},
	})

	loop := agentloop.New(&scriptedClient{}, registry)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "call echo"},
	}, agentloop.RunOptions{Tools: registry.Schemas()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(resp.Message.Content, `"echo":"hello from pkg"`) {
		t.Fatalf("response content = %q", resp.Message.Content)
	}
}
