package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
)

// sseServer answers every request with the given event-stream body.
func sseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startRecorder collects the names OnToolCallStart reports, in order.
type startRecorder struct {
	names []string
}

func (r *startRecorder) options() ChatOptions {
	return ChatOptions{
		OnDelta:         func(string) {},
		OnToolCallStart: func(name string) { r.names = append(r.names, name) },
	}
}

// Each tool_use block is announced once, by name, from its
// content_block_start — before any partial_json for it arrives.
func TestAnthropicChat_StreamAnnouncesToolCallStart(t *testing.T) {
	events := []string{
		"event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":7}}}",
		"event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}",
		"event: content_block_start\ndata: {\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"report_continuity\",\"input\":{}}}",
		"event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"findings\\\":\"}}",
		"event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"[]}\"}}",
		"event: content_block_stop\ndata: {\"index\":1}",
		"event: content_block_start\ndata: {\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_2\",\"name\":\"read_scene\",\"input\":{}}}",
		"event: content_block_delta\ndata: {\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}",
		"event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":3}}",
		"data: [DONE]",
	}
	srv := sseServer(t, strings.Join(events, "\n\n")+"\n\n")
	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	rec := &startRecorder{}
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, rec.options())
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if want := []string{"report_continuity", "read_scene"}; !reflect.DeepEqual(rec.names, want) {
		t.Fatalf("announced %v, want %v", rec.names, want)
	}
	if len(resp.Message.ToolCalls) != 2 || resp.Message.ToolCalls[0].Arguments != `{"findings":[]}` {
		t.Fatalf("tool calls changed: %+v", resp.Message.ToolCalls)
	}
}

// The name arrives on a call's first chunk only; later argument chunks for the
// same index do not announce it again.
func TestOpenAICompatibleChat_StreamAnnouncesToolCallStart(t *testing.T) {
	events := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"write_scene","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"text\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"write_scene","arguments":"\"x\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	srv := sseServer(t, strings.Join(events, "\n\n")+"\n\n")
	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	rec := &startRecorder{}
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, rec.options())
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if want := []string{"write_scene"}; !reflect.DeepEqual(rec.names, want) {
		t.Fatalf("announced %v, want %v", rec.names, want)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Arguments != `{"text":"x"}` {
		t.Fatalf("tool calls changed: %+v", resp.Message.ToolCalls)
	}
}

// Codex announces a call from response.output_item.added, under the name the
// caller registered rather than the sanitized one sent on the wire.
func TestOpenAICodexClient_StreamAnnouncesToolCallStart(t *testing.T) {
	tools := []ToolSchema{{Type: "function", Function: ToolFunctionSchema{Name: "linetta.read", Parameters: []byte(`{"type":"object"}`)}}}
	wire := newOpenAICodexToolNameMap(tools).outbound("linetta.read")
	events := []string{
		`data: {"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_1"}}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"` + wire + `","arguments":""}}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"` + wire + `","arguments":"{}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1}}}`,
		`data: [DONE]`,
	}
	srv := sseServer(t, strings.Join(events, "\n\n")+"\n\n")
	client, err := newOpenAICodexClientWithConfig(
		srv.URL, "gpt-5.3-codex", "oauth", "openai-codex", "", DefaultClientConfig(),
		func() (auth.CodexCredential, error) { return auth.CodexCredential{AccessToken: "token-1"}, nil },
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}
	rec := &startRecorder{}
	opts := rec.options()
	opts.Tools = tools
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, opts); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if want := []string{"linetta.read"}; !reflect.DeepEqual(rec.names, want) {
		t.Fatalf("announced %v, want %v", rec.names, want)
	}
}

// Without the callback a streamed tool call parses exactly as before.
func TestAnthropicChat_StreamToolCallWithoutStartCallback(t *testing.T) {
	events := []string{
		"event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read_scene\",\"input\":{}}}",
		"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}",
		"event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"tool_use\"}}",
		"data: [DONE]",
	}
	srv := sseServer(t, strings.Join(events, "\n\n")+"\n\n")
	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{OnDelta: func(string) {}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "read_scene" {
		t.Fatalf("tool calls: %+v", resp.Message.ToolCalls)
	}
}
