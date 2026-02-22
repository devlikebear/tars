package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/secrets"
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
		Name:        "list_dir",
		Description: "list files",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: `{"path":".","entries":[]}`},
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
					Name:      "list_dir",
					Arguments: `{"path":"."}`,
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
					Name:       "list_dir",
					Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
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

func TestLoop_Run_BlocksToolOutsideInjectedSet(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess"}, nil
	}))
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "exec", Arguments: `{"command":"pwd"}`},
					},
				},
			},
		},
	}

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "현재 디렉토리 경로"},
	}, RunOptions{
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "session_status",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected injected-tool enforcement error")
	}
	if !strings.Contains(err.Error(), "tool not injected for this request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoop_Run_AutoExpandOnce(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess"}, nil
	}))
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})
	reg.Register(tool.Tool{
		Name:        "glob",
		Description: "glob files",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"matches":[]}`}}}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "exec", Arguments: `{"command":"pwd"}`},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_2", Name: "glob", Arguments: `{"pattern":"*.md"}`},
					},
				},
			},
		},
	}

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "tool expand"},
	}, RunOptions{
		MaxIterations:  3,
		AutoExpandOnce: true,
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "session_status",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected second outside tool to be blocked after one-shot expand")
	}
	if !strings.Contains(err.Error(), "tool not injected for this request") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.seenToolCounts) < 2 {
		t.Fatalf("expected at least 2 llm calls, got %v", client.seenToolCounts)
	}
	if client.seenToolCounts[0] != 1 {
		t.Fatalf("expected first call tool count=1, got %d", client.seenToolCounts[0])
	}
	if client.seenToolCounts[1] != 2 {
		t.Fatalf("expected second call tool count=2 after auto-expand, got %d", client.seenToolCounts[1])
	}
}

func TestLoop_Run_AutoExpand_AllowsFirstMissingTool(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess"}, nil
	}))
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "exec", Arguments: `{"command":"pwd"}`},
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

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "tool expand"},
	}, RunOptions{
		MaxIterations:  3,
		AutoExpandOnce: true,
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "session_status",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success with first auto-expand, got %v", err)
	}
	if resp.Message.Content != "done" {
		t.Fatalf("unexpected response: %q", resp.Message.Content)
	}
	if len(client.seenToolCounts) != 2 {
		t.Fatalf("expected 2 llm calls, got %v", client.seenToolCounts)
	}
	if client.seenToolCounts[1] != 2 {
		t.Fatalf("expected second call tool count=2 after auto-expand, got %d", client.seenToolCounts[1])
	}
}

func TestLoop_Run_ExecAliasCallUsesCanonicalTool(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "shell_execute", Arguments: `{"command":"pwd"}`},
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

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "run pwd"},
	}, RunOptions{
		MaxIterations: 3,
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "exec",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected alias call to succeed, got %v", err)
	}
	if resp.Message.Content != "done" {
		t.Fatalf("unexpected response: %q", resp.Message.Content)
	}
}

func TestLoop_Run_StopsOnRepeatedInvalidExecWithoutCommand(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: `{"command":"","exit_code":-1,"duration_ms":0,"message":"command is required; provide JSON like {\"command\":\"pwd\"}"}`},
				},
			}, nil
		},
	})

	invalidExecResp := llm.ChatResponse{
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
			invalidExecResp,
			invalidExecResp,
			invalidExecResp,
		},
	}

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "현재 경로 알려줘"},
	}, RunOptions{
		MaxIterations: 5,
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
		t.Fatal("expected invalid exec retry guard error")
	}
	if !strings.Contains(err.Error(), "repeated invalid exec call") {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.callIndex != 3 {
		t.Fatalf("expected stop at 3 llm calls for invalid exec loop after one auto-correction, got %d", client.callIndex)
	}
}

func TestLoop_Run_AutoCorrectsMissingExecCommand_EmptyObject(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(_ context.Context, params json.RawMessage) (tool.Result, error) {
			var input map[string]string
			_ = json.Unmarshal(params, &input)
			cmd := strings.TrimSpace(input["command"])
			if cmd == "" {
				return tool.Result{
					Content: []tool.ContentBlock{
						{Type: "text", Text: `{"command":"","exit_code":-1,"message":"command is required"}`},
					},
				}, nil
			}
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: fmt.Sprintf(`{"command":"%s","exit_code":0}`, cmd)},
				},
			}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "exec", Arguments: `{}`},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "ok",
				},
			},
		},
	}

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "현재 경로 알려줘"},
	}, RunOptions{
		MaxIterations: 3,
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
	if err != nil {
		t.Fatalf("expected auto-corrected exec call to succeed, got %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("unexpected response: %q", resp.Message.Content)
	}
	secondCall := client.seenInputs[1]
	if len(secondCall) == 0 {
		t.Fatalf("expected second llm call with tool result")
	}
	last := secondCall[len(secondCall)-1]
	if !strings.Contains(last.Content, `"command":"pwd"`) {
		t.Fatalf("expected auto-corrected command pwd in tool result, got %q", last.Content)
	}
}

func TestLoop_Run_AutoCorrectsMissingExecCommand_EmptyStringArguments(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "exec",
		Description: "execute command",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(_ context.Context, params json.RawMessage) (tool.Result, error) {
			var input map[string]string
			_ = json.Unmarshal(params, &input)
			cmd := strings.TrimSpace(input["command"])
			if cmd == "" {
				return tool.Result{
					Content: []tool.ContentBlock{
						{Type: "text", Text: `{"command":"","exit_code":-1,"message":"command is required"}`},
					},
				}, nil
			}
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: fmt.Sprintf(`{"command":"%s","exit_code":0}`, cmd)},
				},
			}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "tool_call_0", Name: "exec", Arguments: ""},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "ok",
				},
			},
		},
	}

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "현재 경로 알려줘"},
	}, RunOptions{
		MaxIterations: 3,
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
	if err != nil {
		t.Fatalf("expected auto-corrected exec call to succeed, got %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("unexpected response: %q", resp.Message.Content)
	}
	secondCall := client.seenInputs[1]
	if len(secondCall) == 0 {
		t.Fatalf("expected second llm call with tool result")
	}
	last := secondCall[len(secondCall)-1]
	if !strings.Contains(last.Content, `"command":"pwd"`) {
		t.Fatalf("expected auto-corrected command pwd in tool result, got %q", last.Content)
	}
}

func TestLoop_Run_RedactsToolResultBeforeLLMAppend(t *testing.T) {
	secrets.ResetForTests()
	reg := tool.NewRegistry()
	secretValue := "sk_live_very_secret_value_1234567890"
	secrets.RegisterNamed("OPENAI_API_KEY", secretValue)

	reg.Register(tool.Tool{
		Name:        "read_file",
		Description: "read file",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{
				Content: []tool.ContentBlock{
					{Type: "text", Text: fmt.Sprintf(`{"token":"%s","ok":true}`, secretValue)},
				},
			}, nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "read_file", Arguments: `{"path":".env"}`},
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

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "read"},
	}, RunOptions{
		MaxIterations: 3,
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "read_file",
					Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(client.seenInputs) < 2 || len(client.seenInputs[1]) == 0 {
		t.Fatalf("expected second llm request with tool result")
	}
	last := client.seenInputs[1][len(client.seenInputs[1])-1]
	if strings.Contains(last.Content, secretValue) {
		t.Fatalf("expected redacted tool result, got %q", last.Content)
	}
}

func TestLoop_Run_FinalizesWithoutToolsWhenMaxIterationsReached(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess-1", HistoryMessages: 1}, nil
	}))

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "session_status", Arguments: `{}`},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_2", Name: "session_status", Arguments: `{}`},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "최종 요약",
				},
			},
		},
	}

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "요약해줘"},
	}, RunOptions{
		MaxIterations: 2,
		Tools: []llm.ToolSchema{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:       "session_status",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("expected fallback finalization success, got %v", err)
	}
	if resp.Message.Content != "최종 요약" {
		t.Fatalf("unexpected final response: %q", resp.Message.Content)
	}
	if len(client.seenToolCounts) != 3 {
		t.Fatalf("expected 3 llm calls, got %d", len(client.seenToolCounts))
	}
	if client.seenToolCounts[2] != 0 {
		t.Fatalf("expected finalization call without tools, got %d", client.seenToolCounts[2])
	}
	if client.seenToolChoice[2] != "none" {
		t.Fatalf("expected finalization tool_choice=none, got %q", client.seenToolChoice[2])
	}
}
