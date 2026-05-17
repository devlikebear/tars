package embodiment

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

func TestMCPTransportDispatchRetriesAndMapsTool(t *testing.T) {
	caller := &fakeMCPToolCaller{
		errs: []error{errors.New("temporary failure"), nil},
	}
	transport := NewMCPTransport(caller, zerolog.New(io.Discard))
	err := transport.Dispatch(context.Background(), ProviderDescriptor{
		Name:      "stackchan",
		Endpoint:  "stackchan",
		Enabled:   true,
		Transport: TransportMCP,
	}, BodyAction{Kind: ActionSpeak, Payload: map[string]any{"text": "hello"}})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %+v, want retry", caller.calls)
	}
	last := caller.calls[1]
	if last.server != "stackchan" || last.tool != "stackchan_speak" || last.args["text"] != "hello" {
		t.Fatalf("unexpected mcp call: %+v", last)
	}
}

func TestMCPTransportDispatchDropsUnsupportedTransport(t *testing.T) {
	transport := NewMCPTransport(&fakeMCPToolCaller{}, zerolog.New(io.Discard))
	err := transport.Dispatch(context.Background(), ProviderDescriptor{
		Name:      "host",
		Enabled:   true,
		Transport: TransportWebhook,
	}, BodyAction{Kind: ActionSpeak, Payload: map[string]any{"text": "hello"}})
	if err == nil {
		t.Fatal("expected unsupported transport error")
	}
}

func TestMCPTransportDispatchTreatsToolErrorAsFailure(t *testing.T) {
	transport := NewMCPTransport(&fakeMCPToolCaller{
		results: []tool.Result{{IsError: true}, {IsError: true}},
	}, zerolog.New(io.Discard))
	err := transport.Dispatch(context.Background(), ProviderDescriptor{
		Name:      "stackchan",
		Enabled:   true,
		Transport: TransportMCP,
	}, BodyAction{Kind: ActionExpress, Payload: map[string]any{"emotion": "happy"}})
	if err == nil {
		t.Fatal("expected tool error result")
	}
}

type fakeMCPToolCaller struct {
	calls   []mcpToolCall
	errs    []error
	results []tool.Result
}

type mcpToolCall struct {
	server string
	tool   string
	args   map[string]any
}

func (f *fakeMCPToolCaller) CallTool(_ context.Context, serverName, toolName string, args map[string]any) (tool.Result, error) {
	f.calls = append(f.calls, mcpToolCall{server: serverName, tool: toolName, args: args})
	idx := len(f.calls) - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return tool.Result{}, f.errs[idx]
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return tool.Result{Content: []tool.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
}
