package tarsclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendMessage_PrintsToolStatusFeedback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"status\",\"phase\":\"before_tool_call\",\"message\":\"executing tool\",\"tool_name\":\"read_file\",\"tool_call_id\":\"call_1\",\"tool_args_preview\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"done\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"s-1\"}\n\n"))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := chatClient{serverURL: server.URL}

	res, err := sendMessage(context.Background(), client, "", "hello", true, false, stdout, stderr)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if strings.TrimSpace(res.SessionID) != "s-1" {
		t.Fatalf("expected session id s-1, got %q", res.SessionID)
	}
	if !strings.Contains(stderr.String(), "executing tool (read_file)") {
		t.Fatalf("expected tool status feedback, got %q", stderr.String())
	}
}
