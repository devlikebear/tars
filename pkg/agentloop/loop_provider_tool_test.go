package agentloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

// liveToolClient reports provider tools through ChatOptions.OnProviderTool
// while "running", then returns resp (or err).
type liveToolClient struct {
	live []llm.ProviderToolEvent
	resp llm.ChatResponse
	err  error
}

func (c *liveToolClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *liveToolClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	for _, evt := range c.live {
		if opts.OnProviderTool != nil {
			opts.OnProviderTool(evt)
		}
	}
	return c.resp, c.err
}

func providerEventLog(events []Event) string {
	var out []string
	for _, evt := range events {
		switch evt.Type {
		case EventProviderTool:
			out = append(out, "start:"+evt.ToolCallID)
		case EventProviderToolResult:
			out = append(out, fmt.Sprintf("result:%s:%s:%s:%q:%v", evt.ToolCallID, evt.ToolName, evt.ToolArgs, evt.ToolResult, evt.ToolIsError))
		case EventAfterLLM, EventLoopError:
			out = append(out, string(evt.Type))
		}
	}
	return strings.Join(out, " ")
}

// Tools reported live surface as they happen — before the LLM call returns —
// and are not repeated from ProviderExecutedTools afterwards. The caller's
// ProviderTool hook still sees each call once, after the call, as before.
func TestLoop_Run_EmitsProviderToolsLiveWithoutDuplicates(t *testing.T) {
	read := llm.ToolCall{ID: "t1", Name: "Read", Arguments: `{"file_path":"a"}`}
	bash := llm.ToolCall{ID: "t2", Name: "Bash", Arguments: `{}`}
	client := &liveToolClient{
		live: []llm.ProviderToolEvent{
			{Call: read},
			{Call: read, Finished: true, Result: "contents", IsError: false},
		},
		resp: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "ok"},
			// t2 was never reported live (a provider without live
			// reporting): it still surfaces once, after the call.
			ProviderExecutedTools: []llm.ToolCall{read, bash},
		},
	}
	var events []Event
	loop := NewLoop(client, tool.NewRegistry(), HookFunc(func(_ context.Context, evt Event) { events = append(events, evt) }))
	var hooked []string
	_, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{
		ProviderTool: func(_ context.Context, evt Event) error {
			hooked = append(hooked, evt.ToolCallID)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := `start:t1 result:t1:Read:{"file_path":"a"}:"contents":false after_llm start:t2`
	if got := providerEventLog(events); got != want {
		t.Fatalf("events = %s\nwant     %s", got, want)
	}
	if strings.Join(hooked, ",") != "t1,t2" {
		t.Fatalf("ProviderTool hook saw %v, want t1,t2", hooked)
	}
}

// A call that fails (timeout, cancel) mid-tool has already surfaced the
// tools it ran, so observers keep them.
func TestLoop_Run_ProviderToolsSurviveFailedCall(t *testing.T) {
	client := &liveToolClient{
		live: []llm.ProviderToolEvent{{Call: llm.ToolCall{ID: "t1", Name: "Bash", Arguments: `{}`}}},
		err:  errors.New("cli timed out"),
	}
	var events []Event
	loop := NewLoop(client, tool.NewRegistry(), HookFunc(func(_ context.Context, evt Event) { events = append(events, evt) }))
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{}); err == nil {
		t.Fatal("expected the call's error")
	}
	if got, want := providerEventLog(events), "start:t1 error"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

// A failed result carries the provider's error flag.
func TestLoop_Run_ProviderToolResultError(t *testing.T) {
	call := llm.ToolCall{ID: "t1", Name: "Bash", Arguments: `{}`}
	client := &liveToolClient{
		live: []llm.ProviderToolEvent{{Call: call}, {Call: call, Finished: true, Result: "exit 1", IsError: true}},
		resp: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}, ProviderExecutedTools: []llm.ToolCall{call}},
	}
	var events []Event
	loop := NewLoop(client, tool.NewRegistry(), HookFunc(func(_ context.Context, evt Event) { events = append(events, evt) }))
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, RunOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := providerEventLog(events), `start:t1 result:t1:Bash:{}:"exit 1":true after_llm`; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}
