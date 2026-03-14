package tarsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChatClientStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("expected auth header, got %q", got)
		}
		if got := r.Header.Get("Tars-Workspace-Id"); got != "" {
			t.Fatalf("expected no workspace header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"status\",\"message\":\"ok\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hel\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"lo\"}\n")
		fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n")
	}))
	defer server.Close()

	client := chatClient{
		serverURL: server.URL,
		apiToken:  "tok",
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

func TestChatClientStream_StreamsDeltaInRealtime(t *testing.T) {
	deltaCh := make(chan string, 4)
	allowFinish := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hel\"}\n")
		flusher.Flush()
		select {
		case <-allowFinish:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"lo\"}\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := chatClient{serverURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resCh := make(chan chatResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := client.stream(ctx, chatRequest{Message: "hi"}, nil, func(text string) {
			deltaCh <- text
		})
		if err != nil {
			errCh <- err
			return
		}
		resCh <- res
	}()

	select {
	case got := <-deltaCh:
		if got != "Hel" {
			t.Fatalf("expected first realtime delta Hel, got %q", got)
		}
		allowFinish <- struct{}{}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected first delta before stream completion")
	}

	select {
	case err := <-errCh:
		t.Fatalf("stream: %v", err)
	case res := <-resCh:
		if res.Assistant != "Hello" {
			t.Fatalf("expected assistant Hello, got %q", res.Assistant)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("stream did not finish")
	}
}

func TestChatClientStream_HTTPErrorReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "invalid_request",
			"message": "invalid request",
		})
	}))
	defer server.Close()

	client := chatClient{serverURL: server.URL}
	_, err := client.stream(context.Background(), chatRequest{Message: "hi"}, nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected apiHTTPError, got %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("expected status=400, got %d", apiErr.Status)
	}
	if strings.TrimSpace(apiErr.Code) != "invalid_request" {
		t.Fatalf("expected code invalid_request, got %q", apiErr.Code)
	}
}

func TestChatClientStream_RetriesWithMainSessionOnStaleSession(t *testing.T) {
	var (
		mu       sync.Mutex
		attempts []chatRequest
		statuses int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat":
			var req chatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			mu.Lock()
			attempts = append(attempts, req)
			attempt := len(attempts)
			mu.Unlock()
			if attempt == 1 {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    "not_found",
					"message": "session not found",
				})
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hello\"}\n")
			fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-main\"}\n")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			statuses++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workspace_dir":   "/tmp/ws",
				"session_count":   1,
				"main_session_id": "sess-main",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := chatClient{serverURL: server.URL}
	res, err := client.stream(context.Background(), chatRequest{Message: "hi", SessionID: "stale-1"}, nil, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if res.SessionID != "sess-main" {
		t.Fatalf("expected recovered session_id sess-main, got %q", res.SessionID)
	}
	if res.Assistant != "Hello" {
		t.Fatalf("expected assistant Hello, got %q", res.Assistant)
	}
	if statuses != 1 {
		t.Fatalf("expected one status lookup, got %d", statuses)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 2 {
		t.Fatalf("expected two chat attempts, got %d", len(attempts))
	}
	if attempts[0].SessionID != "stale-1" {
		t.Fatalf("expected first attempt to use stale session, got %+v", attempts[0])
	}
	if attempts[1].SessionID != "sess-main" {
		t.Fatalf("expected second attempt to use main session, got %+v", attempts[1])
	}
}

func TestChatClientStream_RetriesWithoutSessionWhenNoMainSessionExists(t *testing.T) {
	var (
		mu       sync.Mutex
		attempts []chatRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat":
			var req chatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			mu.Lock()
			attempts = append(attempts, req)
			attempt := len(attempts)
			mu.Unlock()
			if attempt == 1 {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    "not_found",
					"message": "session not found",
				})
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hello\"}\n")
			fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-new\"}\n")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workspace_dir":   "/tmp/ws",
				"session_count":   0,
				"main_session_id": "",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := chatClient{serverURL: server.URL}
	res, err := client.stream(context.Background(), chatRequest{Message: "hi", SessionID: "stale-1"}, nil, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if res.SessionID != "sess-new" {
		t.Fatalf("expected recovered session_id sess-new, got %q", res.SessionID)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 2 {
		t.Fatalf("expected two chat attempts, got %d", len(attempts))
	}
	if attempts[1].SessionID != "" {
		t.Fatalf("expected second attempt without session, got %+v", attempts[1])
	}
}
