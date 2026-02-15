package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type scriptedLLMClient struct {
	responses      []llm.ChatResponse
	callIndex      int
	seenInputs     [][]llm.ChatMessage
	seenToolCounts []int
	seenToolChoice []string
}

func (c *scriptedLLMClient) Ask(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "", nil
}

func (c *scriptedLLMClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	_ = ctx
	copyMsgs := append([]llm.ChatMessage(nil), messages...)
	c.seenInputs = append(c.seenInputs, copyMsgs)
	c.seenToolCounts = append(c.seenToolCounts, len(opts.Tools))
	c.seenToolChoice = append(c.seenToolChoice, opts.ToolChoice)
	resp := c.responses[c.callIndex]
	c.callIndex++
	return resp, nil
}

func TestLoop_Run_WithToolCallAndHooks(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{
			SessionID:       "sess-xyz",
			HistoryMessages: 4,
		}, nil
	}))

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "session_status",
							Arguments: "{}",
						},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "done",
				},
			},
		},
	}

	events := make([]EventType, 0, 8)
	loop := NewLoop(client, reg, HookFunc(func(_ context.Context, evt Event) {
		events = append(events, evt.Type)
	}))

	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "status?"},
	}, RunOptions{
		ToolChoice: "required",
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:        "session_status",
					Description: "status",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if resp.Message.Content != "done" {
		t.Fatalf("unexpected final response: %q", resp.Message.Content)
	}

	if len(client.seenInputs) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(client.seenInputs))
	}
	if len(client.seenToolCounts) != 2 || client.seenToolCounts[0] != 1 || client.seenToolCounts[1] != 1 {
		t.Fatalf("expected tools to be forwarded to each llm call, got %+v", client.seenToolCounts)
	}
	if len(client.seenToolChoice) != 2 || client.seenToolChoice[0] != "required" || client.seenToolChoice[1] != "required" {
		t.Fatalf("expected tool choice to be forwarded to each llm call, got %+v", client.seenToolChoice)
	}

	secondCall := client.seenInputs[1]
	if len(secondCall) == 0 {
		t.Fatalf("expected second llm call messages")
	}
	last := secondCall[len(secondCall)-1]
	if last.Role != "tool" {
		t.Fatalf("expected tool message, got role=%q", last.Role)
	}
	if last.ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id call_1, got %q", last.ToolCallID)
	}

	var parsed tool.SessionStatus
	if err := json.Unmarshal([]byte(last.Content), &parsed); err != nil {
		t.Fatalf("parse tool content: %v", err)
	}
	if parsed.SessionID != "sess-xyz" {
		t.Fatalf("unexpected session id in tool result: %q", parsed.SessionID)
	}

	want := []EventType{
		EventLoopStart,
		EventBeforeLLM,
		EventAfterLLM,
		EventBeforeTool,
		EventAfterTool,
		EventBeforeLLM,
		EventAfterLLM,
		EventLoopEnd,
	}
	if len(events) != len(want) {
		t.Fatalf("unexpected event count: got %d want %d", len(events), len(want))
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("unexpected event at %d: got %q want %q", i, events[i], want[i])
		}
	}
}

func TestLoop_Run_StopsOnRepeatedToolCallPattern(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: `{"command":"","exit_code":-1,"duration_ms":0,"message":"command is required"}`},
				},
			}, nil
		},
	})

	repeatedResp := llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{
				{
					ID:        "call_1",
					Name:      "exec",
					Arguments: `{}`,
				},
			},
		},
	}
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			repeatedResp,
			repeatedResp,
			repeatedResp,
			repeatedResp,
			repeatedResp,
		},
	}

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "현재 디렉토리 경로 알려줘"},
	}, RunOptions{
		MaxIterations: 5,
		ToolChoice:    "required",
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "exec",
					Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected repeated tool call pattern error")
	}
	if !strings.Contains(err.Error(), "repeated tool call pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.callIndex != 3 {
		t.Fatalf("expected early stop at 3 llm calls, got %d", client.callIndex)
	}
}
