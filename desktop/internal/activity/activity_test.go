package activity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/desktop/internal/server"
)

type recorded struct {
	Method, Path, Query, Auth string
	Body                      map[string]any
}

// fakeServer answers the routes the tray uses and records what it was sent.
type fakeServer struct {
	mu       sync.Mutex
	requests []recorded
	status   int
	sessions string
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&rec.Body)
	}
	f.mu.Lock()
	f.requests = append(f.requests, rec)
	status := f.status
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":" no token "}`))
		return
	}
	switch {
	case r.URL.Path == "/v1/chat/activity":
		_, _ = w.Write([]byte(`{"running":[{"session_id":"s1","session_title":"Build","started_at":"2026-09-29T12:00:00Z"}],
			"pending_approvals":[{"request_id":"r1","session_id":"s1","session_title":"Build","tool_name":"Bash","preview":"make","decisions":["allow_once","allow_session","deny"]}]}`))
	case r.URL.Path == "/v1/admin/sessions" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(f.sessions))
	case r.URL.Path == "/v1/admin/sessions" && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"id":"new1","title":"x"}`))
	default:
		_, _ = w.Write([]byte(`{}`))
	}
}

func (f *fakeServer) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func newTestClient(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewClient(server.Config{URL: srv.URL, Token: "user-tok", AdminToken: "admin-tok"}, nil)
}

func TestActivity(t *testing.T) {
	f := &fakeServer{}
	c := newTestClient(t, f)
	snap, err := c.Activity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Running) != 1 || snap.Running[0].Session != "Build" || !snap.Running[0].StartedAt.Equal(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("running = %+v", snap.Running)
	}
	if len(snap.Pending) != 1 || snap.Pending[0].ToolName != "Bash" || len(snap.Pending[0].Decisions) != 3 {
		t.Fatalf("pending = %+v", snap.Pending)
	}
	if got := f.last().Auth; got != "Bearer user-tok" {
		t.Fatalf("activity sent %q, want the user token", got)
	}
}

func TestAnswer(t *testing.T) {
	f := &fakeServer{}
	c := newTestClient(t, f)
	a := Approval{RequestID: "r/1", SessionID: "s1"}
	if err := c.Answer(context.Background(), a, "allow_session"); err != nil {
		t.Fatal(err)
	}
	got := f.last()
	if got.Method != http.MethodPost || got.Path != "/v1/chat/permissions/r/1" || got.Body["session_id"] != "s1" || got.Body["decision"] != "allow_session" {
		t.Fatalf("answer sent %+v", got)
	}
	if err := c.Answer(context.Background(), a, "always"); err == nil {
		t.Fatal("an unknown decision must be refused")
	}
	if err := c.Answer(context.Background(), Approval{RequestID: "r1"}, "deny"); err == nil {
		t.Fatal("an approval without a session must be refused")
	}
}

func TestRecentSessions(t *testing.T) {
	f := &fakeServer{sessions: `[
		{"id":"old","title":"Old","kind":"main","updated_at":"2026-09-01T00:00:00Z"},
		{"id":"worker","title":"W","kind":"worker","updated_at":"2026-09-29T00:00:00Z"},
		{"id":"hidden","title":"H","hidden":true,"updated_at":"2026-09-29T00:00:00Z"},
		{"id":"new","title":"New","updated_at":"2026-09-28T00:00:00Z"},
		{"id":"mid","title":"Mid","kind":"main","updated_at":"2026-09-15T00:00:00Z"}
	]`}
	c := newTestClient(t, f)
	got, err := c.RecentSessions(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "new" || got[1].ID != "mid" {
		t.Fatalf("recent = %+v", got)
	}
	req := f.last()
	if req.Path != "/v1/admin/sessions" || req.Query != "archived=exclude" || req.Auth != "Bearer admin-tok" {
		t.Fatalf("listing sent %+v", req)
	}
}

func TestNewSessionIn(t *testing.T) {
	f := &fakeServer{}
	c := newTestClient(t, f)
	id, err := c.NewSessionIn(context.Background(), "repo", "/work/repo")
	if err != nil || id != "new1" {
		t.Fatalf("new session = %q, %v", id, err)
	}
	f.mu.Lock()
	reqs := append([]recorded(nil), f.requests...)
	f.mu.Unlock()
	if len(reqs) != 2 || reqs[0].Body["title"] != "repo" || reqs[0].Auth != "Bearer admin-tok" {
		t.Fatalf("create sent %+v", reqs)
	}
	put := reqs[1]
	if put.Method != http.MethodPut || put.Path != "/v1/admin/sessions/new1/workdirs" || put.Body["current_dir"] != "/work/repo" {
		t.Fatalf("workdirs sent %+v", put)
	}
	if dirs, _ := put.Body["work_dirs"].([]any); len(dirs) != 1 || dirs[0] != "/work/repo" {
		t.Fatalf("work_dirs = %v", put.Body["work_dirs"])
	}
}

func TestNewSessionInWithoutID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	c := NewClient(server.Config{URL: srv.URL}, srv.Client())
	if _, err := c.NewSessionIn(context.Background(), "x", "/x"); err == nil {
		t.Fatal("a create without an id must fail")
	}
}

func TestAPIErrors(t *testing.T) {
	f := &fakeServer{status: http.StatusUnauthorized}
	c := newTestClient(t, f)
	_, err := c.Activity(context.Background())
	if !IsUnauthorized(err) {
		t.Fatalf("401 = %v", err)
	}
	if !strings.Contains(err.Error(), "401 no token") {
		t.Fatalf("message = %q", err.Error())
	}
	f.mu.Lock()
	f.status = http.StatusInternalServerError
	f.mu.Unlock()
	if _, err := c.RecentSessions(context.Background(), 5); err == nil || IsUnauthorized(err) {
		t.Fatalf("500 = %v", err)
	}
	if IsUnauthorized(errors.New("plain")) {
		t.Fatal("a plain error is not a 401")
	}
	if got := (&APIError{Status: 502}).Error(); got != "tars server: 502" {
		t.Fatalf("bare error = %q", got)
	}
}

func TestStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"keepalive\"}\n\n" +
			": comment\n" +
			"data: not json\n\n" +
			"data: {\"type\":\"notification\",\"category\":\"approval\",\"session_id\":\"s1\",\"request_id\":\"r1\",\"open_path\":\"/console/chat/s1\"}\n\n"))
	}))
	defer srv.Close()

	var got []Event
	c := NewClient(server.Config{URL: srv.URL, Token: "tok"}, srv.Client())
	if err := c.Stream(context.Background(), func(e Event) { got = append(got, e) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Category != "approval" || got[0].RequestID != "r1" || got[0].OpenPath != "/console/chat/s1" {
		t.Fatalf("events = %+v", got)
	}

	noToken := NewClient(server.Config{URL: srv.URL}, srv.Client())
	if err := noToken.Stream(context.Background(), func(Event) {}); !IsUnauthorized(err) {
		t.Fatalf("stream without token = %v", err)
	}
}

func TestTokenChoice(t *testing.T) {
	both := NewClient(server.Config{Token: "u", AdminToken: "a"}, nil)
	if both.token(false) != "u" || both.token(true) != "a" {
		t.Fatal("user token on user routes, admin token on admin routes")
	}
	adminOnly := NewClient(server.Config{AdminToken: "a"}, nil)
	if adminOnly.token(false) != "a" {
		t.Fatal("an admin token stands in for a missing user token")
	}
	userOnly := NewClient(server.Config{Token: "u"}, nil)
	if userOnly.token(true) != "u" {
		t.Fatal("without an admin token, admin routes get the user token and the server decides")
	}
}
