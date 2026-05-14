package tarsserver

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

// providerToolStubClient returns a single response carrying
// ProviderExecutedTools. That single response has no further tool_calls so
// the loop terminates after one iteration.
type providerToolStubClient struct {
	resp llm.ChatResponse
}

func (c *providerToolStubClient) Ask(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return c.resp.Message.Content, nil
}

func (c *providerToolStubClient) Chat(_ context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	return c.resp, nil
}

// TestSetupAgentLoop_ForwardsProviderToolEventToStreamAndTranscript verifies
// the EventProviderTool branch wired into setupAgentLoop:
//  1. The chat stream's sendStatus is invoked with the "provider_tool"
//     status name carrying the upstream tool name + call id + args preview.
//     This is what the console renders as "Claude Code ran Bash(ls)".
//  2. The persisted toolCalls slice gets an entry with the same name/id and
//     a placeholder ToolResult so a session-replay surface knows the tool
//     was upstream-executed without a TARS result.
//  3. No EventBeforeTool / EventAfterTool fires for the same call (those
//     are reserved for TARS-side tool execution).
func TestSetupAgentLoop_ForwardsProviderToolEventToStreamAndTranscript(t *testing.T) {
	client := &providerToolStubClient{
		resp: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "done"},
			ProviderExecutedTools: []llm.ToolCall{
				{ID: "toolu_77", Name: "Bash", Arguments: `{"command":"ls"}`},
			},
		},
	}

	registry := tool.NewRegistry()

	type statusCall struct {
		Name, Detail, ToolName, ToolCallID, ArgsPreview, ResultPreview string
	}
	var statusCalls []statusCall
	sendStatus := func(name, detail, toolName, toolCallID, argsPreview, resultPreview string, _ ...bool) {
		statusCalls = append(statusCalls, statusCall{
			Name: name, Detail: detail, ToolName: toolName, ToolCallID: toolCallID,
			ArgsPreview: argsPreview, ResultPreview: resultPreview,
		})
	}

	loop, toolCalls := setupAgentLoop(
		client,
		registry,
		"sess-test",
		0,
		nil,
		zerolog.Nop(),
		sendStatus,
		nil, // afterTool — must NOT be invoked for provider-executed tools
	)

	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, agent.RunOptions{}); err != nil {
		t.Fatalf("loop run: %v", err)
	}

	// 1) provider_tool stream event surfaced with the right payload.
	var providerEvent *statusCall
	for i := range statusCalls {
		if statusCalls[i].Name == "provider_tool" {
			providerEvent = &statusCalls[i]
			break
		}
	}
	if providerEvent == nil {
		t.Fatalf("expected a provider_tool stream event, got %+v", statusCalls)
	}
	if providerEvent.ToolName != "Bash" {
		t.Fatalf("provider_tool ToolName: got %q want Bash", providerEvent.ToolName)
	}
	if providerEvent.ToolCallID != "toolu_77" {
		t.Fatalf("provider_tool ToolCallID: got %q want toolu_77", providerEvent.ToolCallID)
	}
	if !strings.Contains(providerEvent.ArgsPreview, "ls") {
		t.Fatalf("provider_tool ArgsPreview should reflect args, got %q", providerEvent.ArgsPreview)
	}

	// 2) Persisted transcript entry.
	if len(*toolCalls) != 1 {
		t.Fatalf("expected 1 ToolCallRecord, got %d: %+v", len(*toolCalls), *toolCalls)
	}
	rec := (*toolCalls)[0]
	if rec.ToolName != "Bash" || rec.ToolCallID != "toolu_77" {
		t.Fatalf("transcript record name/id mismatch: %+v", rec)
	}
	if !strings.Contains(rec.ToolResult, "upstream") {
		t.Fatalf("transcript record ToolResult should mark upstream-executed, got %q", rec.ToolResult)
	}
	if rec.ToolIsError {
		t.Fatalf("transcript record should not be flagged error, got %+v", rec)
	}

	// 3) Provider-executed tools must NOT trigger before_tool_call /
	// after_tool_call status events (those are reserved for TARS-side
	// execution and would mislead the console into showing duplicate spans).
	for _, c := range statusCalls {
		if c.Name == "before_tool_call" || c.Name == "after_tool_call" {
			t.Fatalf("provider-executed tools should not emit %s, got %+v", c.Name, statusCalls)
		}
	}
}
