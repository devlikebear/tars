package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
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

// Other clients (the desktop tray) learn which answers a question takes:
// "always allow" only when the prompt names a folder it would cover.
func TestChatActivityOffersAlwaysOnlyWithAFolder(t *testing.T) {
	a := newChatActivity(nil, nil)
	a.observe("s1", map[string]any{"type": "permission_request", "request_id": "r1", "tool_name": "Bash", "always_dir": "/repo"})
	a.observe("s1", map[string]any{"type": "permission_request", "request_id": "r2", "tool_name": "Bash", "always_dir": ""})
	decisions := map[string][]string{}
	for _, p := range a.snapshot().Pending {
		decisions[p.RequestID] = p.Decisions
	}
	if got := decisions["r1"]; !slices.Equal(got, []string{"allow_once", "allow_session", "allow_always", "deny"}) {
		t.Fatalf("with a folder: %v", got)
	}
	if got := decisions["r2"]; !slices.Equal(got, []string{"allow_once", "allow_session", "deny"}) {
		t.Fatalf("without a folder: %v", got)
	}
}

// Unattended runs (#970) ask through the ops queue. The activity lists
// their questions in queued_approvals (#1033), apart from the chat
// permission requests, so no client answers them through the chat API.
func TestChatActivityListsQueuedUnattendedApprovals(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	other, err := f.store.Create("quiet")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", approvalPreviewRunes+40)
	waiting, err := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: f.session, Source: "cron", RunLabel: "nightly build", ToolName: "exec", Preview: long, Reason: "runs a command"})
	if err != nil {
		t.Fatal(err)
	}
	answered, _ := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: other.ID, Source: "telegram", ToolName: "exec"})
	if err := f.ops.ReviewToolPermission(answered.ID, true); err != nil {
		t.Fatal(err)
	}

	tooling := defaultChatToolingOptions()
	tooling.Unattended = f.perms
	srv := httptest.NewServer(newChatAPIHandlerWithRuntimeConfig(f.root, f.store, &permissionAskingClient{}, nil, zerolog.New(io.Discard), 0, nil, "", tooling))
	t.Cleanup(srv.Close)

	snap := getActivity(t, srv.URL)
	if len(snap.Running) != 0 || len(snap.Pending) != 0 {
		t.Fatalf("an unattended question is not a chat turn or a chat permission: %+v", snap)
	}
	if len(snap.Queued) != 1 {
		t.Fatalf("queued = %+v", snap.Queued)
	}
	q := snap.Queued[0]
	if q.ApprovalID != waiting.ID || q.SessionID != f.session || q.Session != "nightly" || q.Source != "cron" || q.RunLabel != "nightly build" || q.ToolName != "exec" || q.Reason != "runs a command" {
		t.Fatalf("queued approval = %+v", q)
	}
	if !q.RequestedAt.Equal(waiting.RequestedAt) {
		t.Fatalf("requested_at = %v, want %v", q.RequestedAt, waiting.RequestedAt)
	}
	if n := len([]rune(q.Preview)); n != approvalPreviewRunes || !strings.HasSuffix(q.Preview, "…") {
		t.Fatalf("preview is capped like a chat approval's, got %d runes", n)
	}

	if err := f.ops.ReviewToolPermission(waiting.ID, false); err != nil {
		t.Fatal(err)
	}
	if snap := getActivity(t, srv.URL); len(snap.Queued) != 0 {
		t.Fatalf("after the review = %+v", snap.Queued)
	}
}

func TestChatActivityQueuedApprovalsAreOptional(t *testing.T) {
	read := func(activity *chatActivity) string {
		t.Helper()
		rec := httptest.NewRecorder()
		handleChatActivity(rec, httptest.NewRequest(http.MethodGet, "/v1/chat/activity", nil), activity)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		return rec.Body.String()
	}
	// Older clients read only running and pending_approvals; every array
	// is present, never null.
	for name, activity := range map[string]*chatActivity{"nil tracker": nil, "no source": newChatActivity(nil, nil)} {
		body := read(activity)
		for _, field := range []string{`"running":[]`, `"pending_approvals":[]`, `"queued_approvals":[]`} {
			if !strings.Contains(body, field) {
				t.Fatalf("%s: %s missing from %s", name, field, body)
			}
		}
	}

	failing := newChatActivity(nil, nil)
	failing.queued = func() ([]chatQueuedApproval, error) { return nil, errors.New("ops unavailable") }
	if body := read(failing); !strings.Contains(body, `"queued_approvals":[]`) {
		t.Fatalf("an unreadable queue lists nothing, got %s", body)
	}

	early := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	sorted := newChatActivity(nil, nil)
	sorted.queued = func() ([]chatQueuedApproval, error) {
		return []chatQueuedApproval{
			{ApprovalID: "b", SessionID: "s", Session: "kept", RequestedAt: early.Add(time.Minute)},
			{ApprovalID: "a", SessionID: "s", RequestedAt: early},
		}, nil
	}
	got := sorted.queuedApprovals()
	if len(got) != 2 || got[0].ApprovalID != "a" || got[1].Session != "kept" {
		t.Fatalf("oldest first = %+v", got)
	}
}
