package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

// gateFunc adapts a function to ToolAuthorizer.
type gateFunc func(context.Context, ToolCallRequest) (ToolDecision, error)

func (f gateFunc) Authorize(ctx context.Context, req ToolCallRequest) (ToolDecision, error) {
	return f(ctx, req)
}

// gatedLoop runs one write_file call followed by a final answer, counting
// how often the tool actually ran.
func gatedLoop(t *testing.T, gate ToolAuthorizer) (*scriptedLLMClient, *int, []Event, error) {
	t.Helper()
	runs := 0
	reg := tool.NewRegistry()
	reg.Register(tool.Tool{
		Name:        "write_file",
		Description: "write a file",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(context.Context, json.RawMessage) (tool.Result, error) {
			runs++
			return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: "wrote a.txt"}}}, nil
		},
	})
	client := &scriptedLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "write_file", Arguments: `{"path":"a.txt"}`}}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "finished"}},
	}}
	var events []Event
	loop := NewLoop(client, reg, HookFunc(func(_ context.Context, evt Event) { events = append(events, evt) }))
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "write it"}}, RunOptions{
		Tools:          reg.Schemas(),
		ToolAuthorizer: gate,
	})
	return client, &runs, events, err
}

func TestLoop_ToolAuthorizerAllowRunsTheTool(t *testing.T) {
	var seen ToolCallRequest
	_, runs, _, err := gatedLoop(t, gateFunc(func(_ context.Context, req ToolCallRequest) (ToolDecision, error) {
		seen = req
		return ToolDecision{Allow: true}, nil
	}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if *runs != 1 {
		t.Fatalf("tool ran %d times, want 1", *runs)
	}
	if seen.ToolName != "write_file" || seen.ToolCallID != "call_1" || seen.ToolArgs != `{"path":"a.txt"}` {
		t.Fatalf("gate saw %+v", seen)
	}
}

// A denial is an answer, not a failure: the model learns the call was
// refused and the turn goes on.
func TestLoop_ToolAuthorizerDenyReportsToTheModelAndContinues(t *testing.T) {
	client, runs, events, err := gatedLoop(t, gateFunc(func(context.Context, ToolCallRequest) (ToolDecision, error) {
		return ToolDecision{Message: "not in this folder"}, nil
	}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if *runs != 0 {
		t.Fatalf("a denied tool ran %d times", *runs)
	}
	if client.callIndex != 2 {
		t.Fatalf("the turn stopped after the denial (llm calls = %d)", client.callIndex)
	}
	second := client.seenInputs[1]
	last := second[len(second)-1]
	if last.Role != "tool" || last.ToolCallID != "call_1" || !strings.Contains(last.Content, "not in this folder") {
		t.Fatalf("tool result sent to the model = %+v", last)
	}

	var after *Event
	for i := range events {
		if events[i].Type == EventAfterTool {
			after = &events[i]
		}
	}
	if after == nil || !after.ToolIsError || !strings.Contains(after.ToolResult, "not in this folder") {
		t.Fatalf("after_tool event = %+v", after)
	}
}

func TestLoop_ToolAuthorizerDenyWithoutMessageUsesDefault(t *testing.T) {
	client, _, _, err := gatedLoop(t, gateFunc(func(context.Context, ToolCallRequest) (ToolDecision, error) {
		return ToolDecision{}, nil
	}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	second := client.seenInputs[1]
	if content := second[len(second)-1].Content; !strings.Contains(content, "denied") {
		t.Fatalf("tool result = %q", content)
	}
}

// A gate that cannot decide (the person left, the turn was cancelled) stops
// the turn rather than guessing.
func TestLoop_ToolAuthorizerErrorStopsTheTurn(t *testing.T) {
	_, runs, _, err := gatedLoop(t, gateFunc(func(context.Context, ToolCallRequest) (ToolDecision, error) {
		return ToolDecision{}, context.Canceled
	}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if *runs != 0 {
		t.Fatalf("the tool ran %d times without a decision", *runs)
	}
}
