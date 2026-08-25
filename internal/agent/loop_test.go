package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/tool"
)

type scriptedLLMClient struct {
	responses      []llm.ChatResponse
	callIndex      int
	seenInputs     [][]llm.ChatMessage
	seenToolCounts []int
	seenToolChoice []string
	seenResumeIDs  []string
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
	c.seenToolChoice = append(c.seenToolChoice, opts.ToolChoice.String())
	c.seenResumeIDs = append(c.seenResumeIDs, opts.ResumeSessionID)
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
	var beforeTool Event
	var afterTool Event
	loop := NewLoop(client, reg, HookFunc(func(_ context.Context, evt Event) {
		events = append(events, evt.Type)
		if evt.Type == EventBeforeTool {
			beforeTool = evt
		}
		if evt.Type == EventAfterTool {
			afterTool = evt
		}
	}))

	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "status?"},
	}, RunOptions{
		ToolChoice: llm.ToolChoiceRequired(),
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
	if beforeTool.ToolArgs != "{}" {
		t.Fatalf("expected before_tool args to be preserved, got %q", beforeTool.ToolArgs)
	}
	if afterTool.ToolArgs != "{}" {
		t.Fatalf("expected after_tool args to be preserved, got %q", afterTool.ToolArgs)
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
		ToolChoice:    llm.ToolChoiceRequired(),
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
		ToolChoice: llm.ToolChoiceAuto(),
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
	// The finalization call keeps the tool list so it stays in the same cache
	// lineage as the loop iterations; suppression is tool_choice=none. What
	// this test guards is the outcome — finalization yields text — not the
	// mechanism. See the ignores-none fallback test below for the case where a
	// provider does not honor the suppression.
	if client.seenToolCounts[2] == 0 {
		t.Fatalf("finalization should keep the tools it was called with, got %d", client.seenToolCounts[2])
	}
	if client.seenToolChoice[2] != "none" {
		t.Fatalf("expected finalization tool_choice=none, got %q", client.seenToolChoice[2])
	}
}

// A provider that ignores tool_choice=none would hand back another tool call
// with no text, which the loop cannot use — before this fallback existed that
// turn degraded into the max-iterations error instead of an answer.
func TestLoop_Run_FinalizationRetriesWithoutToolsWhenProviderIgnoresNone(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess"}, nil
	}))

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "call_1", Name: "session_status", Arguments: `{}`},
			}}},
			// Finalization attempt: provider ignores none and calls a tool.
			{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "call_2", Name: "session_status", Arguments: `{}`},
			}}},
			// Retry with the tools removed: now it answers.
			{Message: llm.ChatMessage{Role: "assistant", Content: "fallback answer"}},
		},
	}

	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{
		MaxIterations: 1,
		Tools:         reg.Schemas(),
		ToolChoice:    llm.ToolChoiceAuto(),
	})
	if err != nil {
		t.Fatalf("fallback should still produce an answer, got %v", err)
	}
	if resp.Message.Content != "fallback answer" {
		t.Fatalf("unexpected final content: %q", resp.Message.Content)
	}
	if len(client.seenToolCounts) != 3 {
		t.Fatalf("expected loop + finalization + retry = 3 calls, got %v", client.seenToolCounts)
	}
	if client.seenToolCounts[1] == 0 {
		t.Fatalf("first finalization attempt should carry tools, got %v", client.seenToolCounts)
	}
	if client.seenToolCounts[2] != 0 {
		t.Fatalf("retry must drop the tools to make a text answer structural, got %v", client.seenToolCounts)
	}
}

type testRecordingEmitter struct {
	events []testEmittedLine
}

type testEmittedLine struct {
	toolCallID string
	stream     string
	text       string
}

func (r *testRecordingEmitter) EmitToolLine(toolCallID, stream, text string) {
	r.events = append(r.events, testEmittedLine{toolCallID, stream, text})
}

// TestLoop_BindsLineEmitterToToolCallID verifies that when a chat-level
// LineEmitter is in the loop ctx, the per-call ToolOutputStreamer the
// tool sees is bound to the active tool_call_id.
func TestLoop_BindsLineEmitterToToolCallID(t *testing.T) {
	reg := tool.NewRegistry()
	var seenStreamer tool.ToolOutputStreamer
	reg.Register(tool.Tool{
		Name:        "ctx_probe",
		Description: "captures streamer from ctx",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (tool.Result, error) {
			seenStreamer = tool.ToolOutputStreamerFromContext(ctx)
			if seenStreamer != nil {
				seenStreamer.EmitLine(tool.StreamStdout, "from-tool")
			}
			return tool.JSONTextResult(map[string]string{"ok": "1"}, false), nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "abc-42", Name: "ctx_probe", Arguments: "{}"}}}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
		},
	}

	rec := &testRecordingEmitter{}
	ctx := tool.WithLineEmitter(context.Background(), rec)
	loop := NewLoop(client, reg)

	_, err := loop.Run(ctx, []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{
		Tools: []llm.ToolSchema{{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:       "ctx_probe",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
		ToolChoice: llm.ToolChoiceAuto(),
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if seenStreamer == nil {
		t.Fatalf("expected tool to receive a ToolOutputStreamer from ctx")
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 streamed line via emitter, got %d", len(rec.events))
	}
	if rec.events[0].toolCallID != "abc-42" {
		t.Fatalf("expected toolCallID=abc-42, got %q", rec.events[0].toolCallID)
	}
	if rec.events[0].text != "from-tool" || rec.events[0].stream != tool.StreamStdout {
		t.Fatalf("unexpected event %+v", rec.events[0])
	}
}

func TestLoop_NoEmitterMeansNoStreamerInToolCtx(t *testing.T) {
	reg := tool.NewRegistry()
	var seenStreamer tool.ToolOutputStreamer
	reg.Register(tool.Tool{
		Name:        "ctx_probe",
		Description: "captures streamer from ctx",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (tool.Result, error) {
			seenStreamer = tool.ToolOutputStreamerFromContext(ctx)
			return tool.JSONTextResult(map[string]string{"ok": "1"}, false), nil
		},
	})

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "x", Name: "ctx_probe", Arguments: "{}"}}}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
		},
	}

	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{
		Tools: []llm.ToolSchema{{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:       "ctx_probe",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
		ToolChoice: llm.ToolChoiceAuto(),
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if seenStreamer != nil {
		t.Fatalf("expected no streamer when no emitter in ctx, got %v", seenStreamer)
	}
}

// TestLoop_Run_ThreadsResumeSessionID verifies that:
//   - The first iteration receives ChatOptions.ResumeSessionID = caller intent.
//   - When the provider returns a SessionID on the response, subsequent
//     iterations adopt it (covers the fresh-session case: caller had no ID,
//     provider mints one, iter 2 onwards resumes that fresh session).
func TestLoop_Run_ThreadsResumeSessionID(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "tars-sess", HistoryMessages: 0}, nil
	}))

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				// Iter 1: provider mints a fresh upstream session id.
				SessionID: "upstream-fresh-1",
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "session_status", Arguments: "{}"},
					},
				},
			},
			{
				// Iter 2: provider returns the same id (resumed).
				SessionID: "upstream-fresh-1",
				Message:   llm.ChatMessage{Role: "assistant", Content: "done"},
			},
		},
	}
	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{
		Tools: []llm.ToolSchema{{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:       "session_status",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
		ToolChoice: llm.ToolChoiceAuto(),
		// Caller passes empty — this is a brand-new TARS session.
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(client.seenResumeIDs) != 2 {
		t.Fatalf("expected 2 chat calls, got %d", len(client.seenResumeIDs))
	}
	if client.seenResumeIDs[0] != "" {
		t.Fatalf("iter 1 should see empty resume id (fresh session), got %q", client.seenResumeIDs[0])
	}
	if client.seenResumeIDs[1] != "upstream-fresh-1" {
		t.Fatalf("iter 2 should adopt upstream id from iter 1 response, got %q", client.seenResumeIDs[1])
	}
}

// TestLoop_Run_ProviderExecutedToolsDoNotTriggerLocalExecution verifies that
// when the response carries tools the provider has already executed (e.g. the
// claude-code-cli stream's tool_use blocks, which Claude Code itself ran
// internally), the loop does NOT try to re-execute them through TARS' own
// tool registry. Without this guarantee, a chat session using
// claude-code-cli would surface a "tool not allowed" error on every turn
// where Claude Code touched a file or ran bash, since Claude Code's tool
// names ("Read", "Edit", "Bash") don't match TARS' registry. ToolCalls left
// on Message keeps the "model wants TARS to execute this" semantic for other
// providers; provider-executed tools live in a separate field and are
// observation-only.
func TestLoop_Run_ProviderExecutedToolsDoNotTriggerLocalExecution(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{Role: "assistant", Content: "read it and done"},
				// Mimics claude-code-cli stream: Claude already executed
				// these tools internally, so the loop must NOT route them
				// through TARS' tool registry.
				ProviderExecutedTools: []llm.ToolCall{
					{ID: "toolu_01", Name: "Read", Arguments: `{"file_path":"/tmp/a.txt"}`},
					{ID: "toolu_02", Name: "Bash", Arguments: `{"command":"ls"}`},
				},
			},
		},
	}
	loop := NewLoop(client, reg)
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "read it"}}, RunOptions{
		// Empty allowed tools — if the loop tried to dispatch Read/Bash
		// locally, it would fall into the blocked-tool error path.
	})
	if err != nil {
		t.Fatalf("loop should ignore provider-executed tools, got error: %v", err)
	}
	if resp.Message.Content != "read it and done" {
		t.Fatalf("unexpected response content: %q", resp.Message.Content)
	}
	// Loop should have called the provider exactly once — no extra iterations.
	if client.callIndex != 1 {
		t.Fatalf("expected single chat call, got %d", client.callIndex)
	}
}

// TestLoop_Run_EmitsProviderToolEvent verifies that provider-executed tools
// surface as agent.Event entries so the chat handler / console / ops can
// audit what the upstream agent actually did. Event type is
// EventProviderTool, fired once per ProviderExecutedTools entry just before
// EventAfterLLM so observers see them inline with the iteration that
// produced them.
func TestLoop_Run_EmitsProviderToolEvent(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{Role: "assistant", Content: "ok"},
				ProviderExecutedTools: []llm.ToolCall{
					{ID: "toolu_01", Name: "Read", Arguments: `{"file_path":"/tmp/a.txt"}`},
				},
			},
		},
	}
	events := []Event{}
	loop := NewLoop(client, reg, HookFunc(func(_ context.Context, evt Event) {
		events = append(events, evt)
	}))
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{}); err != nil {
		t.Fatalf("loop run: %v", err)
	}
	var providerToolEvts []Event
	for _, evt := range events {
		if evt.Type == EventProviderTool {
			providerToolEvts = append(providerToolEvts, evt)
		}
	}
	if len(providerToolEvts) != 1 {
		t.Fatalf("expected 1 EventProviderTool, got %d (all events: %+v)", len(providerToolEvts), events)
	}
	got := providerToolEvts[0]
	if got.ToolName != "Read" {
		t.Fatalf("expected ToolName=Read, got %q", got.ToolName)
	}
	if got.ToolCallID != "toolu_01" {
		t.Fatalf("expected ToolCallID=toolu_01, got %q", got.ToolCallID)
	}
	if got.ToolArgs == "" || !strings.Contains(got.ToolArgs, "/tmp/a.txt") {
		t.Fatalf("expected ToolArgs to include /tmp/a.txt, got %q", got.ToolArgs)
	}
}

// TestLoop_Run_HonorsCallerResumeSessionID verifies that when the caller seeds
// a ResumeSessionID, iter 1 receives it (the resume case across user turns).
func TestLoop_Run_HonorsCallerResumeSessionID(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				SessionID: "carried",
				Message:   llm.ChatMessage{Role: "assistant", Content: "ack"},
			},
		},
	}
	loop := NewLoop(client, reg)
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "second turn"}}, RunOptions{
		ResumeSessionID: "carried",
	})
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(client.seenResumeIDs) != 1 || client.seenResumeIDs[0] != "carried" {
		t.Fatalf("expected iter 1 to receive caller resume id 'carried', got %v", client.seenResumeIDs)
	}
}

// The turn's final call used to drop the tool list. Providers render tools
// ahead of messages into one prefix-matched cache key, so a tools-absent
// request lands in a different cache lineage than the tool-bearing loop
// iterations it follows — it cannot read what they just wrote, and its own
// cache write is paid at a premium for an entry the next iteration cannot use.
//
// Keeping the tools and suppressing them with tool_choice=none puts the call
// back in the turn's lineage. It also makes ToolChoiceNone actually reach the
// wire: every provider emits tool_choice only when tools are present, so with
// Tools=nil the "none" was silently dropped.
func TestLoop_Run_FinalCallKeepsToolsAndSuppressesWithToolChoiceNone(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{SessionID: "sess"}, nil
	}))

	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "call-1", Name: "session_status", Arguments: "{}"},
					},
				},
				StopReason: "tool_use",
			},
			{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
		},
	}

	loop := NewLoop(client, reg)
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "status please"},
	}, RunOptions{
		Tools:         reg.Schemas(),
		ToolChoice:    llm.ToolChoiceAuto(),
		MaxIterations: 1,
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(client.seenToolCounts) < 2 {
		t.Fatalf("expected the loop to reach its final call, got %v", client.seenToolCounts)
	}
	last := len(client.seenToolCounts) - 1
	if client.seenToolCounts[last] == 0 {
		t.Fatalf("final call must still carry the tool list so it shares the turn's cache prefix, got %v", client.seenToolCounts)
	}
	if client.seenToolCounts[last] != client.seenToolCounts[0] {
		t.Fatalf("final call tool count %d should match the loop's %d — a different tool set is a different cache prefix",
			client.seenToolCounts[last], client.seenToolCounts[0])
	}
	if got := client.seenToolChoice[last]; got != "none" {
		t.Fatalf("final call must suppress tool use with tool_choice=none, got %q", got)
	}
}
