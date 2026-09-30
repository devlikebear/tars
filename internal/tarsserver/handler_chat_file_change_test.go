package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

// A native provider's write_file call reports its change on the chat stream
// as soon as the call finishes (#1032).
func TestChatAPI_WriteFileEmitsFileChangeEvent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mockClient := &mockLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
				ID:        "call_write",
				Name:      "write_file",
				Arguments: `{"path":"notes/a.txt","content":"one\ntwo\n"}`,
			}}}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "Wrote it."}},
		},
	}
	handler := newChatAPIHandler(root, store, mockClient, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"write a note"}`))
	req.Header.Set("Content-Type", "application/json")
	// High-risk file tools are injected for admins only.
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	got := fileChangeEventsAfterTool(t, rec.Body.String(), "call_write")
	if len(got) != 1 {
		t.Fatalf("expected one file_change event, got %d: %q", len(got), rec.Body.String())
	}
	evt := got[0]
	if evt["tool_call_id"] != "call_write" || evt["op"] != "create" || evt["additions"] != float64(2) || evt["deletions"] != float64(0) {
		t.Fatalf("unexpected file_change event: %v", evt)
	}
	if path, _ := evt["path"].(string); !strings.HasSuffix(path, "notes/a.txt") {
		t.Fatalf("unexpected path: %v", evt["path"])
	}
	hunks, _ := evt["hunks"].([]any)
	if len(hunks) != 1 {
		t.Fatalf("expected one hunk, got %v", evt["hunks"])
	}
	hunk := hunks[0].(map[string]any)
	if hunk["new_start"] != float64(1) || hunk["new_lines"] != float64(2) {
		t.Fatalf("unexpected hunk: %v", hunk)
	}
}

// fileChangeEventsAfterTool returns the stream's file_change events, failing
// if one arrives before the named tool call's after_tool_call status.
func fileChangeEventsAfterTool(t *testing.T, body, toolCallID string) []map[string]any {
	t.Helper()
	var got []map[string]any
	finished := false
	for _, evt := range sseEvents(t, body) {
		if evt["type"] == "status" && evt["phase"] == "after_tool_call" && evt["tool_call_id"] == toolCallID {
			finished = true
		}
		if evt["type"] != "file_change" {
			continue
		}
		if !finished {
			t.Fatal("file_change arrived before the tool call finished")
		}
		got = append(got, evt)
	}
	return got
}

func TestChatStreamWriterFileChangeShape(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := newChatStreamWriter(rec, "sess", zerolog.New(io.Discard))
	feed := newChatTurnFeed()
	stream.feed = feed
	stream.fileChange("call_1", tool.FileChange{Path: ""})
	stream.fileChange(" call_1 ", tool.FileChange{Path: "img.png", Op: "modify", Binary: true, Truncated: true})
	var nilStream *chatStreamWriter
	nilStream.fileChange("x", tool.FileChange{Path: "a"})

	events, _, _, _, _ := feed.since(0)
	if len(events) != 1 {
		t.Fatalf("expected one event (empty path dropped), got %d", len(events))
	}
	var evt map[string]any
	if err := json.Unmarshal(events[0], &evt); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type": "file_change", "session_id": "sess", "tool_call_id": "call_1", "path": "img.png",
		"op": "modify", "additions": float64(0), "deletions": float64(0), "binary": true, "truncated": true,
	}
	for k, v := range want {
		if evt[k] != v {
			t.Fatalf("%s: got %v, want %v (event %v)", k, evt[k], v, evt)
		}
	}
	if _, ok := evt["hunks"]; ok {
		t.Fatalf("hunks must be omitted when empty: %v", evt)
	}
	if !strings.Contains(rec.Body.String(), `"type":"file_change"`) {
		t.Fatal("event not written to the response")
	}
}
