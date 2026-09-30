package agentloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

func TestLoop_Run_CarriesToolFileChangesOnAfterTool(t *testing.T) {
	reg := tool.NewRegistry()
	change := tool.FileChange{Path: "a.txt", Op: "create", Additions: 1}
	reg.Register(tool.Tool{
		Name:       "edit_something",
		Parameters: json.RawMessage(`{"type":"object"}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{
				Content:     []tool.ContentBlock{{Type: "text", Text: "ok"}},
				FileChanges: []tool.FileChange{change},
			}, nil
		},
	})
	client := &scriptedLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "edit_something", Arguments: "{}"}}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
	}}
	var got []tool.FileChange
	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{
		Tools: []llm.ToolSchema{{Type: "function", Function: llm.ToolFunctionSchema{Name: "edit_something", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		AfterTool: func(_ context.Context, evt Event) error {
			if evt.ToolCallID == "call_1" {
				got = evt.ToolFileChanges
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 1 || got[0].Path != change.Path || got[0].Additions != 1 {
		t.Fatalf("file changes: %+v", got)
	}
	last := client.seenInputs[1][len(client.seenInputs[1])-1]
	if strings.Contains(last.Content, "a.txt") {
		t.Fatalf("file changes must not reach the model: %q", last.Content)
	}
}
