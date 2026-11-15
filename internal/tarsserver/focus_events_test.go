package tarsserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

func TestFocusEventFingerprintsFollowEachPipelineAndItsSessions(t *testing.T) {
	pipelines := []focuspipeline.Pipeline{
		{SessionID: "p1", QASessionID: "p1-qa", Goal: "one"},
		{SessionID: "p2", Goal: "two"},
		{SessionID: " "}, // no session: left out
	}
	base := focusEventFingerprints(pipelines, chatActivitySnapshot{}, nil)
	if len(base) != 2 || base["p1"] == "" || base["p2"] == "" {
		t.Fatalf("fingerprints = %v", base)
	}
	if again := focusEventFingerprints(pipelines, chatActivitySnapshot{}, nil); !mapsEqual(base, again) {
		t.Fatal("the same facts must give the same fingerprints")
	}

	cases := map[string]struct {
		snap   chatActivitySnapshot
		queued []chatQueuedApproval
		edit   func([]focuspipeline.Pipeline)
		want   []string
	}{
		"a turn runs in the Q&A session": {
			snap: chatActivitySnapshot{Running: []chatRunningSession{{SessionID: "p1-qa"}}},
			want: []string{"p1"},
		},
		"a question waits": {
			snap: chatActivitySnapshot{Pending: []chatPendingApproval{{SessionID: "p2", RequestID: "r1"}}},
			want: []string{"p2"},
		},
		"a tool call is queued": {
			queued: []chatQueuedApproval{{SessionID: "p1", ApprovalID: "a1"}},
			want:   []string{"p1"},
		},
		"another session's turn": {
			snap:   chatActivitySnapshot{Running: []chatRunningSession{{SessionID: "chat"}}},
			queued: []chatQueuedApproval{{SessionID: "chat", ApprovalID: "a2"}},
			want:   nil,
		},
		"the pipeline changed": {
			edit: func(p []focuspipeline.Pipeline) { p[1].OpenGate = focuspipeline.GatePlan },
			want: []string{"p2"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			list := slices.Clone(pipelines)
			if tc.edit != nil {
				tc.edit(list)
			}
			got := focusEventChanges(base, focusEventFingerprints(list, tc.snap, tc.queued))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("changes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFocusEventChangesListsAddedAndRemoved(t *testing.T) {
	got := focusEventChanges(map[string]string{"a": "1", "b": "1", "gone": "1"}, map[string]string{"a": "1", "b": "2", "new": "1"})
	if want := []string{"b", "gone", "new"}; !slices.Equal(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type focusEventsFixture struct {
	hub      *focusEventHub
	store    *focuspipeline.Store
	activity *chatActivity

	mu     sync.Mutex
	queued []chatQueuedApproval
}

func newFocusEventsFixture(t *testing.T) *focusEventsFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	sessions := session.NewStore(root)
	f := &focusEventsFixture{}
	f.hub = newFocusEventHub(sessions, nil)
	f.hub.settle = 5 * time.Millisecond
	f.hub.queued = func() ([]chatQueuedApproval, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.queued, nil
	}
	f.store = focusStoreFor(sessions)
	f.activity = newChatActivity(sessions, nil)
	f.hub.bindActivity(f.activity)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f.hub.start(ctx)
	return f
}

// waitBaseline waits until the hub has read once with a client connected,
// so the next change is compared with something.
func (f *focusEventsFixture) waitBaseline(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.hub.mu.Lock()
		ready := f.hub.last != nil
		f.hub.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the hub never read a baseline")
}

func nextFocusEvent(t *testing.T, events <-chan focusEvent) focusEvent {
	t.Helper()
	select {
	case evt, ok := <-events:
		if !ok {
			t.Fatal("the stream closed")
		}
		return evt
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	return focusEvent{}
}

func TestFocusEventHubTellsClientsWhichPipelineChanged(t *testing.T) {
	f := newFocusEventsFixture(t)
	if err := f.store.Save(focuspipeline.Pipeline{SessionID: "p1", QASessionID: "p1-qa"}); err != nil {
		t.Fatal(err)
	}
	events, stop := f.hub.subscribe()
	defer stop()
	f.waitBaseline(t)

	// The pipeline file changes.
	if _, _, err := f.store.Update("p1", func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.OpenGate = focuspipeline.GatePlan
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	if evt := nextFocusEvent(t, events); evt != (focusEvent{Type: "pipeline", SessionID: "p1"}) {
		t.Fatalf("event = %+v", evt)
	}

	// A turn starts in the Q&A session, and a question is asked there.
	end := f.activity.begin("p1-qa")
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}
	f.activity.observe("p1-qa", map[string]any{"type": "permission_request", "request_id": "r1", "tool_name": "Bash"})
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}
	f.activity.observe("p1-qa", map[string]any{"type": "permission_resolved", "request_id": "r1"})
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}
	end()
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}

	// A turn in a session that is no pipeline's says nothing.
	f.activity.begin("chat")()
	select {
	case evt := <-events:
		t.Fatalf("a chat turn sent %+v", evt)
	case <-time.After(50 * time.Millisecond):
	}

	// The pipeline is removed.
	if err := f.store.Delete("p1"); err != nil {
		t.Fatal(err)
	}
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}
}

func TestFocusEventHubQueuedToolCalls(t *testing.T) {
	f := newFocusEventsFixture(t)
	if err := f.store.Save(focuspipeline.Pipeline{SessionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	events, stop := f.hub.subscribe()
	defer stop()
	f.waitBaseline(t)

	f.mu.Lock()
	f.queued = []chatQueuedApproval{{SessionID: "p1", ApprovalID: "a1"}}
	f.mu.Unlock()
	f.hub.poke()
	if evt := nextFocusEvent(t, events); evt.SessionID != "p1" {
		t.Fatalf("event = %+v", evt)
	}
}

func TestFocusEventHubReadsNothingWithoutClients(t *testing.T) {
	f := newFocusEventsFixture(t)
	events, stop := f.hub.subscribe()
	f.waitBaseline(t)
	stop()
	if _, open := <-events; open {
		t.Fatal("a removed client's channel must close")
	}
	f.hub.poke()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.hub.mu.Lock()
		last := f.hub.last
		f.hub.mu.Unlock()
		if last == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hub kept its baseline with no client")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestFocusEventHubDropsAClientThatFallsBehind(t *testing.T) {
	hub := &focusEventHub{subs: map[int]chan focusEvent{}, wake: make(chan struct{}, 1)}
	events, stop := hub.subscribe()
	defer stop()
	hub.mu.Lock()
	for i := 0; i <= focusEventsBuffer; i++ {
		hub.sendLocked(focusEvent{Type: "pipeline", SessionID: "p"})
	}
	hub.mu.Unlock()
	count := 0
	for range events {
		count++
	}
	if count != focusEventsBuffer {
		t.Fatalf("read %d events before the close, want %d", count, focusEventsBuffer)
	}
}

func TestFocusEventsHandlerStreamsPipelineChanges(t *testing.T) {
	f := newFocusEventsFixture(t)
	if err := f.store.Save(focuspipeline.Pipeline{SessionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newFocusEventsHandler(f.hub))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/focus/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	read := func() focusEvent {
		t.Helper()
		for lines.Scan() {
			line := lines.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var evt focusEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err != nil {
				t.Fatal(err)
			}
			return evt
		}
		t.Fatal("stream ended")
		return focusEvent{}
	}
	if evt := read(); evt.Type != "connected" {
		t.Fatalf("first event = %+v", evt)
	}
	f.waitBaseline(t)
	if _, _, err := f.store.Update("p1", func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.Goal = "now"
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	if evt := read(); evt != (focusEvent{Type: "pipeline", SessionID: "p1"}) {
		t.Fatalf("event = %+v", evt)
	}
}

func TestFocusEventsHandlerRefusesOtherMethodsAndNoHub(t *testing.T) {
	rec := httptest.NewRecorder()
	newFocusEventsHandler(&focusEventHub{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/focus/events", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	newFocusEventsHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/focus/events", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no hub = %d", rec.Code)
	}
}

func TestFocusEventHubEndsStreamsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hub := newFocusEventHub(session.NewStore(t.TempDir()), nil)
	hub.start(ctx)
	events, stop := hub.subscribe()
	defer stop()
	cancel()
	select {
	case _, open := <-events:
		if open {
			t.Fatal("expected the stream to close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown left the stream open")
	}
}
