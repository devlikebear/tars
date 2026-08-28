package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

func TestLoopBeforeToolPersistenceFailurePreventsEffect(t *testing.T) {
	executions := 0
	loop, schemas := recoveryTestLoop(t, &executions)
	_, err := loop.Run(context.Background(), recoveryInitialMessages(), RunOptions{
		MaxIterations: 2,
		Tools:         schemas,
		BeforeTool: func(context.Context, Event) error {
			return errors.New("persist pending receipt")
		},
	})
	if err == nil || executions != 0 {
		t.Fatalf("before-tool failure err=%v executions=%d", err, executions)
	}
}

func TestLoopAfterToolPersistenceFailureLeavesSingleEffect(t *testing.T) {
	executions := 0
	loop, schemas := recoveryTestLoop(t, &executions)
	_, err := loop.Run(context.Background(), recoveryInitialMessages(), RunOptions{
		MaxIterations: 2,
		Tools:         schemas,
		AfterTool: func(context.Context, Event) error {
			return errors.New("commit effect receipt")
		},
	})
	if err == nil || executions != 1 {
		t.Fatalf("after-tool failure err=%v executions=%d", err, executions)
	}
}

func TestLoopReplaysCommittedToolResultWithoutRepeatingEffect(t *testing.T) {
	executions := 0
	var after Event
	loop, schemas := recoveryTestLoop(t, &executions)
	resp, err := loop.Run(context.Background(), recoveryInitialMessages(), RunOptions{
		MaxIterations: 2,
		Tools:         schemas,
		ReplayToolResult: func(_ context.Context, request ToolReplayRequest) (ToolReplayResult, bool) {
			if request.ToolName != "send_message" || request.ToolArgs != `{"channel":"ops","text":"hello"}` {
				t.Fatalf("replay request: %+v", request)
			}
			return ToolReplayResult{Result: `{"message_id":"msg-42"}`, ReceiptID: "efx-42"}, true
		},
		AfterTool: func(_ context.Context, event Event) error {
			after = event
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run replay: %v", err)
	}
	if executions != 0 {
		t.Fatalf("replay repeated external effect %d time(s)", executions)
	}
	if !after.ToolReplayed || after.ToolReceiptID != "efx-42" || after.ToolResult != `{"message_id":"msg-42"}` {
		t.Fatalf("replayed after-tool event: %+v", after)
	}
	if resp.Message.Content != "done" {
		t.Fatalf("response: %+v", resp)
	}
}

func recoveryTestLoop(t *testing.T, executions *int) (*Loop, []llm.ToolSchema) {
	t.Helper()
	registry := tool.NewRegistry()
	registry.Register(tool.Tool{
		Name:        "send_message",
		Description: "send one external message",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Recovery:    tool.ToolRecoveryPolicy{EffectClass: tool.ToolEffectUnsafe},
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			*executions++
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"message_id":"new"}`}}}, nil
		},
	})
	client := &scriptedLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID: "call-1", Name: "send_message", Arguments: `{"channel":"ops","text":"hello"}`,
		}}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
	}}
	return NewLoop(client, registry), registry.Schemas()
}

func recoveryInitialMessages() []llm.ChatMessage {
	return []llm.ChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "send"}}
}
