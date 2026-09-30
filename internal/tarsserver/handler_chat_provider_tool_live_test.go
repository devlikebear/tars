package tarsserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

// liveProviderToolClient reports provider tools live through
// ChatOptions.OnProviderTool, the way claude-code-cli does while its
// subprocess runs, then returns resp or err.
type liveProviderToolClient struct {
	live []llm.ProviderToolEvent
	resp llm.ChatResponse
	err  error
}

func (c *liveProviderToolClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *liveProviderToolClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	for _, evt := range c.live {
		if opts.OnProviderTool != nil {
			opts.OnProviderTool(evt)
		}
	}
	return c.resp, c.err
}

var (
	liveRead = llm.ToolCall{ID: "toolu_r", Name: "Read", Arguments: `{"file_path":"/tmp/a.txt"}`}
	liveBash = llm.ToolCall{ID: "toolu_b", Name: "Bash", Arguments: `{"command":"make test"}`}
)

// A provider tool streams as provider_tool when it starts and
// provider_tool_result when its result arrives, and its transcript record
// carries that result.
func TestSetupAgentLoop_StreamsLiveProviderToolResult(t *testing.T) {
	client := &liveProviderToolClient{
		live: []llm.ProviderToolEvent{
			{Call: liveBash},
			{Call: liveBash, Finished: true, Result: "FAIL ./pkg", IsError: true},
		},
		resp: llm.ChatResponse{
			Message:               llm.ChatMessage{Role: "assistant", Content: "done"},
			ProviderExecutedTools: []llm.ToolCall{liveBash},
		},
	}
	type statusCall struct {
		Name, ToolCallID, Result string
		IsError                  bool
	}
	var calls []statusCall
	sendStatus := func(name, _, _, toolCallID, _, resultPreview string, isError ...bool) {
		if !strings.HasPrefix(name, "provider_tool") {
			return
		}
		calls = append(calls, statusCall{Name: name, ToolCallID: toolCallID, Result: resultPreview, IsError: len(isError) > 0 && isError[0]})
	}
	loop, records := setupAgentLoop(client, tool.NewRegistry(), "sess", 0, nil, zerolog.Nop(), sendStatus, nil)
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, agent.RunOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []statusCall{
		{Name: "provider_tool", ToolCallID: "toolu_b"},
		{Name: "provider_tool_result", ToolCallID: "toolu_b", Result: "FAIL ./pkg", IsError: true},
	}
	if len(calls) != len(want) {
		t.Fatalf("statuses = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("status %d = %+v, want %+v", i, calls[i], want[i])
		}
	}
	if len(*records) != 1 {
		t.Fatalf("records = %+v, want one", *records)
	}
	if rec := (*records)[0]; rec.ToolCallID != "toolu_b" || rec.ToolResult != "FAIL ./pkg" || !rec.ToolIsError {
		t.Fatalf("record = %+v", rec)
	}
}

func newLiveProviderToolHandler(t *testing.T, client llm.Client) (http.Handler, *session.Store) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	return newChatAPIHandler(root, store, client, zerolog.New(io.Discard)), store
}

func postChat(t *testing.T, handler http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"fix the tests"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Body.String()
}

func onlyTranscriptToolMessages(t *testing.T, store *session.Store) []session.Message {
	t.Helper()
	sessions, err := store.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, err %v", sessions, err)
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sessions[0].ID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var tools []session.Message
	for _, m := range messages {
		if m.Role == "tool" {
			tools = append(tools, m)
		}
	}
	return tools
}

// A reopened session shows each provider tool once, with its result.
func TestChatAPI_ProviderToolsPersistOnceWithResults(t *testing.T) {
	handler, store := newLiveProviderToolHandler(t, &liveProviderToolClient{
		live: []llm.ProviderToolEvent{
			{Call: liveRead},
			{Call: liveRead, Finished: true, Result: "file contents"},
		},
		resp: llm.ChatResponse{
			Message:               llm.ChatMessage{Role: "assistant", Content: "read it"},
			ProviderExecutedTools: []llm.ToolCall{liveRead},
		},
	})
	body := postChat(t, handler)
	if !strings.Contains(body, `"phase":"provider_tool_result"`) {
		t.Fatalf("expected a provider_tool_result event, got %q", body)
	}
	tools := onlyTranscriptToolMessages(t, store)
	if len(tools) != 1 || tools[0].ToolCallID != "toolu_r" || tools[0].Content != "file contents" || tools[0].ToolIsError {
		t.Fatalf("transcript tools = %+v", tools)
	}
}

// A turn cut off by a timeout keeps the tools it ran: the finished one with
// its result, the unfinished one marked as having no result.
func TestChatAPI_ProviderToolsSurviveFailedTurn(t *testing.T) {
	handler, store := newLiveProviderToolHandler(t, &liveProviderToolClient{
		live: []llm.ProviderToolEvent{
			{Call: liveRead},
			{Call: liveRead, Finished: true, Result: "file contents"},
			{Call: liveBash},
		},
		err: errors.New("claude-code-cli request failed: cli timed out: no output for 15m0s"),
	})
	body := postChat(t, handler)
	if !strings.Contains(body, `"tool_call_id":"toolu_b"`) {
		t.Fatalf("expected the unfinished tool on the stream, got %q", body)
	}
	tools := onlyTranscriptToolMessages(t, store)
	if len(tools) != 2 {
		t.Fatalf("transcript tools = %+v, want both calls", tools)
	}
	if tools[0].ToolCallID != "toolu_r" || tools[0].Content != "file contents" {
		t.Fatalf("finished tool = %+v", tools[0])
	}
	if tools[1].ToolCallID != "toolu_b" || tools[1].Content != providerToolInterruptedResult {
		t.Fatalf("unfinished tool = %+v", tools[1])
	}
}
