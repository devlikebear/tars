package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeSSEBuffer(t *testing.T) {
	buf := "data: {\"type\":\"status\",\"message\":\"calling llm\"}\n" +
		"data: {\"type\":\"delta\",\"text\":\"hello\"}\n" +
		"data: {\"type\":\"done\",\"session_id\":\"s1\"}\n"
	events, err := decodeSSEBuffer(strings.NewReader(buf))
	if err != nil {
		t.Fatalf("decodeSSEBuffer: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Type != "delta" || events[1].Text != "hello" {
		t.Fatalf("unexpected delta event: %+v", events[1])
	}
}

func TestChatClientStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("expected auth header, got %q", got)
		}
		if got := r.Header.Get("Tars-Workspace-Id"); got != "ws-a" {
			t.Fatalf("expected workspace header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"status\",\"message\":\"ok\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hel\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"lo\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n")
	}))
	defer server.Close()

	client := chatClient{
		serverURL:   server.URL,
		apiToken:    "tok",
		workspaceID: "ws-a",
	}
	res, err := client.stream(context.Background(), chatRequest{Message: "hi"}, nil, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if res.SessionID != "sess-1" {
		t.Fatalf("expected session_id sess-1, got %q", res.SessionID)
	}
	if res.Assistant != "Hello" {
		t.Fatalf("expected assistant Hello, got %q", res.Assistant)
	}
}
