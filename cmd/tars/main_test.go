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

func TestRun_ChatREPL(t *testing.T) {
	callCount := 0
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("hello\n/quit\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"reply\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-repl\"}\n\n"))
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if callCount != 1 {
		t.Fatalf("expected one chat api call, got %d", callCount)
	}
	if !strings.Contains(stdout.String(), "Entering chat REPL") {
		t.Fatalf("expected repl banner, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "reply") {
		t.Fatalf("expected repl response in stdout, got %q", stdout.String())
	}
}

func TestRun_ChatREPL_ReusesSessionID(t *testing.T) {
	callCount := 0
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("first\nsecond\n/exit\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if callCount == 1 {
			if req.SessionID != "" {
				t.Fatalf("expected empty session_id for first message, got %q", req.SessionID)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"first-reply\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-42\"}\n\n"))
			return
		}

		if req.SessionID != "sess-42" {
			t.Fatalf("expected session_id sess-42 for second message, got %q", req.SessionID)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"second-reply\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-42\"}\n\n"))
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if callCount != 2 {
		t.Fatalf("expected two chat api calls, got %d", callCount)
	}
	if !strings.Contains(stdout.String(), "first-reply") || !strings.Contains(stdout.String(), "second-reply") {
		t.Fatalf("expected both replies in stdout, got %q", stdout.String())
	}
}
