package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

type captureRunnerToolsClient struct {
	toolNames []string
}

func (c *captureRunnerToolsClient) Ask(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (c *captureRunnerToolsClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	names := make([]string, 0, len(opts.Tools))
	for _, schema := range opts.Tools {
		names = append(names, schema.Function.Name)
	}
	c.toolNames = names
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "ok",
		},
	}, nil
}

func TestAgentPromptRunnerWithTools_IncludesExtraTools(t *testing.T) {
	client := &captureRunnerToolsClient{}
	extra := tool.Tool{
		Name:        "telegram_send",
		Description: "send telegram message",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{}, nil
		},
	}
	runner := newAgentPromptRunnerWithTools(t.TempDir(), client, 2, zerolog.New(io.Discard), extra)
	if runner == nil {
		t.Fatalf("expected non-nil runner")
	}
	if _, err := runner(context.Background(), "cron:test", "hello", nil); err != nil {
		t.Fatalf("runner call failed: %v", err)
	}
	found := false
	for _, name := range client.toolNames {
		if name == "telegram_send" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected telegram_send in tool schemas, got %+v", client.toolNames)
	}
}
