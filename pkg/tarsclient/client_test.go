package tarsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_StreamChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("expected auth header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"status\",\"message\":\"ok\"}\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"Hel\"}\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"lo\"}\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n")
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL, APIToken: "tok"})
	res, err := client.StreamChat(context.Background(), ChatRequest{Message: "hi"}, nil, nil)
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	if res.SessionID != "sess-1" {
		t.Fatalf("expected session_id sess-1, got %q", res.SessionID)
	}
	if res.Assistant != "Hello" {
		t.Fatalf("expected assistant Hello, got %q", res.Assistant)
	}
}

func TestClient_DoAndConvenienceMethods(t *testing.T) {
	var adminAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace_dir": "/tmp/ws", "session_count": 3})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "s1", "title": "chat"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/extensions/reload":
			adminAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"reloaded": true, "version": 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL, APIToken: "user-token", AdminAPIToken: "admin-token"})
	var status StatusInfo
	if err := client.Do(context.Background(), http.MethodGet, "/v1/status", nil, &status); err != nil {
		t.Fatalf("Do status: %v", err)
	}
	if status.WorkspaceDir != "/tmp/ws" || status.SessionCount != 3 {
		t.Fatalf("unexpected status: %+v", status)
	}

	sessions, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}

	reload, err := client.ReloadExtensions(context.Background())
	if err != nil {
		t.Fatalf("ReloadExtensions: %v", err)
	}
	if !reload.Reloaded || reload.Version != 2 {
		t.Fatalf("unexpected reload response: %+v", reload)
	}
	if strings.TrimSpace(adminAuth) != "Bearer admin-token" {
		t.Fatalf("expected admin token auth header, got %q", adminAuth)
	}
}

func TestClient_StreamEvents_StopsOnUnauthorized(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/stream" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "unauthorized", "message": "unauthorized"})
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL})
	errCh := make(chan error, 2)
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		client.StreamEvents(ctx, nil, func(err error) { errCh <- err })
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(700 * time.Millisecond):
		t.Fatal("StreamEvents should stop quickly on unauthorized")
	}

	select {
	case err := <-errCh:
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected APIError, got %T (%v)", err, err)
		}
		if apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("expected status=401, got %d", apiErr.Status)
		}
	default:
		t.Fatal("expected unauthorized error callback")
	}

	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected single request for unauthorized stream, got %d", requests)
	}
}
