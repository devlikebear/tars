package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func getActivity(t *testing.T, url string) chatActivitySnapshot {
	t.Helper()
	resp, err := http.Get(url + "/v1/chat/activity")
	if err != nil {
		t.Fatalf("get activity: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activity status %d", resp.StatusCode)
	}
	var snap chatActivitySnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	return snap
}

// A client other than the request running the turn — the desktop tray —
// sees the turn running, sees it wait for approval, answers through the chat
// API, and sees both clear when the turn ends.
func TestChatActivityShowsARunningTurnWaitingForApproval(t *testing.T) {
	client := &permissionAskingClient{}
	srv := newPermissionTestServer(t, client)
	if err := srv.store.SetTitle(srv.session, "Fix the build"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	events := srv.startChat(t, true)
	prompt := waitForEvent(t, events, "permission_request")
	requestID, _ := prompt["request_id"].(string)

	snap := getActivity(t, srv.server.URL)
	if len(snap.Running) != 1 || snap.Running[0].SessionID != srv.session || snap.Running[0].Session != "Fix the build" {
		t.Fatalf("running = %+v", snap.Running)
	}
	if len(snap.Pending) != 1 {
		t.Fatalf("pending = %+v", snap.Pending)
	}
	p := snap.Pending[0]
	if p.RequestID != requestID || p.SessionID != srv.session || p.ToolName != "Bash" || p.Preview != "touch hello.txt" || p.Reason != "writes a file" || p.Session != "Fix the build" {
		t.Fatalf("pending approval = %+v", p)
	}

	if code := srv.answer(t, p.RequestID, "allow_once"); code != http.StatusOK {
		t.Fatalf("answer status %d", code)
	}
	waitForEvent(t, events, "done")
	// The handler returns right after the last event; give it a moment.
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap = getActivity(t, srv.server.URL)
		if len(snap.Running) == 0 && len(snap.Pending) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("activity after the turn = %+v", snap)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestChatActivityEndpointRejectsOtherMethods(t *testing.T) {
	srv := newPermissionTestServer(t, &permissionAskingClient{})
	resp, err := http.Post(srv.server.URL+"/v1/chat/activity", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status %d", resp.StatusCode)
	}
	if snap := getActivity(t, srv.server.URL); len(snap.Running) != 0 || len(snap.Pending) != 0 {
		t.Fatalf("idle activity = %+v", snap)
	}
}

// A new question is announced on the event stream once, with what the
// desktop needs to offer approval: the session, the request, and a link.
func TestChatActivityAnnouncesEachQuestion(t *testing.T) {
	var mu sync.Mutex
	var got []notificationEvent
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("Deploy")
	if err != nil {
		t.Fatal(err)
	}
	tooling := defaultChatToolingOptions()
	tooling.Notify = func(_ context.Context, evt notificationEvent) {
		mu.Lock()
		got = append(got, evt)
		mu.Unlock()
	}
	server := httptest.NewServer(newChatAPIHandlerWithRuntimeConfig(root, store, &permissionAskingClient{}, nil, zerolog.New(io.Discard), 0, nil, "", tooling))
	t.Cleanup(server.Close)
	srv := permissionTestServer{server: server, store: store, session: sess.ID, root: root}
	events := srv.startChat(t, true)
	requestID := waitForEvent(t, events, "permission_request")["request_id"].(string)
	srv.answer(t, requestID, "deny")
	waitForEvent(t, events, "done")

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("notifications = %+v, want one", got)
	}
	evt := got[0]
	if evt.Category != "approval" || evt.SessionID != sess.ID || evt.RequestID != requestID ||
		evt.OpenPath != "/console/chat/"+sess.ID || evt.Title != "Approval needed: Deploy" || evt.Message != "Bash: touch hello.txt" {
		t.Fatalf("notification = %+v", evt)
	}
}

func TestChatActivityTracker(t *testing.T) {
	a := newChatActivity(nil, nil)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }

	endFirst := a.begin("s1")
	endSecond := a.begin("s1")
	a.observe("s1", map[string]any{"type": "permission_request", "request_id": "r1", "tool_name": "write_file", "input": map[string]any{"path": "notes.md"}})
	a.observe("s1", map[string]any{"type": "status", "request_id": "ignored"})
	a.observe("s1", map[string]any{"type": "permission_request"}) // no id
	if snap := a.snapshot(); len(snap.Running) != 1 || len(snap.Pending) != 1 || snap.Pending[0].Preview != "notes.md" {
		t.Fatalf("snapshot = %+v", snap)
	}
	endFirst()
	endFirst() // idempotent
	if snap := a.snapshot(); len(snap.Running) != 1 {
		t.Fatalf("one of two overlapping requests ended; still running: %+v", snap)
	}
	endSecond()
	if snap := a.snapshot(); len(snap.Running) != 0 || len(snap.Pending) != 0 {
		t.Fatalf("an ended turn leaves nothing behind: %+v", snap)
	}

	a.observe("s2", map[string]any{"type": "permission_request", "request_id": "r2", "tool_name": "Bash", "input": json.RawMessage(`{"command":"` + strings.Repeat("x", 300) + `"}`)})
	if p := a.snapshot().Pending[0]; len([]rune(p.Preview)) != approvalPreviewRunes || !strings.HasSuffix(p.Preview, "…") {
		t.Fatalf("long preview = %d runes", len([]rune(p.Preview)))
	}
	a.observe("s2", map[string]any{"type": "permission_resolved", "request_id": "r2"})
	if len(a.snapshot().Pending) != 0 {
		t.Fatal("a resolved question is still pending")
	}

	var nilTracker *chatActivity
	nilTracker.observe("s", map[string]any{"type": "permission_request", "request_id": "x"})
	nilTracker.begin("s")()
	if snap := nilTracker.snapshot(); snap.Running == nil || snap.Pending == nil {
		t.Fatal("a nil tracker must still encode empty lists")
	}
}

func TestApprovalPreviewFallsBackToTheDescription(t *testing.T) {
	if got := approvalPreview(map[string]any{"description": "  Create hello.txt "}); got != "Create hello.txt" {
		t.Fatalf("preview = %q", got)
	}
	if got := approvalPreview(map[string]any{"input": json.RawMessage(`{"url":"https://example.com"}`)}); got != "https://example.com" {
		t.Fatalf("preview = %q", got)
	}
}
