package main

import (
	"context"
	"encoding/json"
	"fmt"

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
		return llm.ChatResponse{
			Message: llm.ChatMessage{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:        "call_echo",
					Name:      opts.Tools[0].Function.Name,
					Arguments: `{"text":"hello from a tiny agent"}`,
				}},
			},
		}, nil
	}
	return llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: messages[len(messages)-1].Content},
	}, nil
}

func main() {
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
		panic(err)
	}
	fmt.Println(resp.Message.Content)
}
