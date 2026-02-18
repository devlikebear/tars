package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteCommand_NewAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s-new", "title": "nightly"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace_dir": "/tmp/ws", "session_count": 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	handled, session, err := executeCommand(context.Background(), runtime, "/new nightly", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/new: %v", err)
	}
	if !handled || session != "s-new" {
		t.Fatalf("expected handled with new session, handled=%t session=%q", handled, session)
	}
	if !strings.Contains(stderr.String(), "session=s-new") {
		t.Fatalf("expected session output in stderr, got %q", stderr.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/status", session, stdout, stderr)
	if err != nil {
		t.Fatalf("/status: %v", err)
	}
	if !strings.Contains(stdout.String(), "workspace=/tmp/ws") {
		t.Fatalf("expected workspace status output, got %q", stdout.String())
	}
}

func TestExecuteCommand_CompactRequiresSession(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: "http://127.0.0.1:43180"}
	_, _, err := executeCommand(context.Background(), runtime, "/compact", "", stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "active session") {
		t.Fatalf("expected active session error, got %v", err)
	}
}
