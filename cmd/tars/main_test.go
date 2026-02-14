package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_DefaultShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "CLI client for TARS") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_HeartbeatRunOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/heartbeat/run-once" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"hello from server"}`))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"heartbeat", "run-once", "--server-url", server.URL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "hello from server") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_ChatMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.SessionID != "sess-1" {
			t.Fatalf("expected session_id sess-1, got %q", req.SessionID)
		}
		if req.Message != "hello" {
			t.Fatalf("expected message hello, got %q", req.Message)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"Hello \"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"from TARS\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n\n"))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"chat", "-m", "hello", "--session", "sess-1", "--server-url", server.URL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Hello from TARS") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_ChatMessage_RequiresMessage(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"chat"}, stdout, stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code when message is missing")
	}
}
