package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/tools"
)

// probeTool fails while failing() says so. Each call gets different arguments
// from the test, so the repeated-call check never fires first.
func probeTool(failing func() bool) tools.Tool {
	return tools.Tool{
		Name:       "probe",
		Parameters: json.RawMessage(`{"type":"object"}`),
		Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
			if failing() {
				return tools.Result{Content: []tools.ContentBlock{{Type: "text", Text: "boom"}}, IsError: true}, nil
			}
			return tools.Result{Content: []tools.ContentBlock{{Type: "text", Text: "ok"}}}, nil
		},
	}
}

func probeCall(n int) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
		{ID: fmt.Sprintf("call_%d", n), Name: "probe", Arguments: fmt.Sprintf(`{"n":%d}`, n)},
	}}}
}

func lastInput(t *testing.T, client *scriptedLLMClient) llm.ChatMessage {
	t.Helper()
	if len(client.seenInputs) == 0 {
		t.Fatal("no llm calls recorded")
	}
	last := client.seenInputs[len(client.seenInputs)-1]
	return last[len(last)-1]
}

func TestLoop_Run_IterationLimitTellsTheModelWhyToolsAreOff(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(probeTool(func() bool { return false }))
	client := &scriptedLLMClient{responses: []llm.ChatResponse{
		probeCall(1),
		probeCall(2),
		{Message: llm.ChatMessage{Role: "assistant", Content: "done so far"}},
	}}

	resp, err := NewLoop(client, reg).Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{MaxIterations: 2, Tools: reg.Schemas(), ToolChoice: llm.ToolChoiceAuto()})
	if err != nil || resp.Message.Content != "done so far" {
		t.Fatalf("expected a final answer at the limit, got %q err=%v", resp.Message.Content, err)
	}
	notice := lastInput(t, client)
	if notice.Role != "user" || !strings.Contains(notice.Content, "limit of 2 tool rounds") || !strings.Contains(notice.Content, "nothing is broken") {
		t.Fatalf("the final call should explain the limit, got %+v", notice)
	}
}

func TestLoop_Run_StopsAfterConsecutiveToolErrors(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(probeTool(func() bool { return true }))
	responses := make([]llm.ChatResponse, 0, consecutiveToolErrorLimit+1)
	for n := 1; n <= consecutiveToolErrorLimit; n++ {
		responses = append(responses, probeCall(n))
	}
	responses = append(responses, llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "it keeps failing"}})
	client := &scriptedLLMClient{responses: responses}

	resp, err := NewLoop(client, reg).Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{MaxIterations: 100, Tools: reg.Schemas(), ToolChoice: llm.ToolChoiceAuto()})
	if err != nil || resp.Message.Content != "it keeps failing" {
		t.Fatalf("expected a final answer after the error streak, got %q err=%v", resp.Message.Content, err)
	}
	if got := len(client.seenInputs); got != consecutiveToolErrorLimit+1 {
		t.Fatalf("expected %d tool rounds and one final call, got %d calls", consecutiveToolErrorLimit, got)
	}
	notice := lastInput(t, client)
	if !strings.Contains(notice.Content, fmt.Sprintf("last %d tool calls in a row failed", consecutiveToolErrorLimit)) {
		t.Fatalf("the final call should name the error streak, got %q", notice.Content)
	}
}

func TestLoop_Run_ASuccessResetsTheErrorStreak(t *testing.T) {
	calls := 0
	reg := tools.NewRegistry()
	// Every call fails except the one in the middle of the run.
	reg.Register(probeTool(func() bool {
		calls++
		return calls != consecutiveToolErrorLimit
	}))
	rounds := 2*consecutiveToolErrorLimit - 2
	responses := make([]llm.ChatResponse, 0, rounds+1)
	for n := 1; n <= rounds; n++ {
		responses = append(responses, probeCall(n))
	}
	responses = append(responses, llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "finished"}})
	client := &scriptedLLMClient{responses: responses}

	resp, err := NewLoop(client, reg).Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{MaxIterations: 100, Tools: reg.Schemas(), ToolChoice: llm.ToolChoiceAuto()})
	if err != nil || resp.Message.Content != "finished" {
		t.Fatalf("expected the run to finish on its own, got %q err=%v", resp.Message.Content, err)
	}
	if got := lastInput(t, client); got.Role != "tool" {
		t.Fatalf("no stop notice expected when the streak was broken, got %+v", got)
	}
}
