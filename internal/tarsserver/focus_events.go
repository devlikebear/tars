package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
)

// The focus event stream (docs/decisions/mobile-remote.md §4):
// GET /v1/focus/events tells a client the moment a focus pipeline changes,
// so a phone's inbox shows a gate the moment it opens instead of on its
// next 30-second read.
//
// Phones do not get /v1/events/stream: most of its events carry no session
// to hold a phone to. This stream says only which pipeline changed —
// {"type":"pipeline","session_id":…}, no title, no text — and the phone
// reads the pipeline list and the chat activity again, which are already
// scoped to focus sessions. So the stream shows a phone nothing it could
// not already read.
//
// What counts as a change: the pipeline file (every save goes through
// focuspipeline.Store, which tells WatchChanges), and the turns and
// questions of the pipeline's session and its Q&A session — a turn starting
// or ending, a permission question asked or answered, an unattended tool
// call queued or closed. Each of those pokes the hub; the hub waits a
// moment for more, then compares one fingerprint per pipeline with the last
// and sends an event for each that differs. Nothing is read while no client
// is connected.

const (
	// focusEventsSettle gathers the pokes of one change (a turn ends: the
	// activity, then the pipeline save) into one read.
	focusEventsSettle = 300 * time.Millisecond
	// focusEventsSafety reads again with no poke, for a change that reached
	// the files some other way. Only while a client is connected.
	focusEventsSafety = 30 * time.Second
	// focusEventsKeepalive keeps proxies and the phone's reader from
	// closing an idle stream.
	focusEventsKeepalive = 15 * time.Second
	// focusEventsBuffer is how many events wait for a slow client; a client
	// that falls further behind is dropped and reads everything on
	// reconnect.
	focusEventsBuffer = 32
)

// focusEvent is one message on the stream.
type focusEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
}

type focusEventHub struct {
	pipelines *focuspipeline.Store
	// queued lists unattended turns' tool calls waiting in the ops queue.
	queued func() ([]chatQueuedApproval, error)
	settle time.Duration
	safety time.Duration

	mu sync.Mutex
	// activity is the chat handler's running turns and questions, bound
	// once the chat handler exists.
	activity func() chatActivitySnapshot
	subs     map[int]chan focusEvent
	nextSub  int
	// last is each pipeline's fingerprint at the last read; nil until a
	// read happens with a client connected.
	last map[string]string

	wake chan struct{}
}

func newFocusEventHub(sessions *session.Store, unattended *unattendedPermissions) *focusEventHub {
	h := &focusEventHub{
		pipelines: focusStoreFor(sessions),
		settle:    focusEventsSettle,
		safety:    focusEventsSafety,
		subs:      map[int]chan focusEvent{},
		wake:      make(chan struct{}, 1),
	}
	if unattended != nil {
		h.queued = unattended.queuedApprovals
		unattended.changed = h.poke
	}
	return h
}

// bindActivity gives the hub the chat handler's activity and has the
// activity poke the hub on every change.
func (h *focusEventHub) bindActivity(activity *chatActivity) {
	if h == nil || activity == nil {
		return
	}
	h.mu.Lock()
	h.activity = activity.snapshot
	h.mu.Unlock()
	activity.mu.Lock()
	activity.changed = h.poke
	activity.mu.Unlock()
}

// poke says something may have changed. It never blocks.
func (h *focusEventHub) poke() {
	if h == nil {
		return
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// start runs the hub until ctx ends.
func (h *focusEventHub) start(ctx context.Context) {
	if h == nil || h.pipelines == nil {
		return
	}
	stop := focuspipeline.WatchChanges(h.pipelines.Dir(), func(string) { h.poke() })
	go func() {
		defer stop()
		safety := time.NewTicker(h.safety)
		defer safety.Stop()
		for {
			select {
			case <-ctx.Done():
				h.closeAll()
				return
			case <-h.wake:
				timer := time.NewTimer(h.settle)
				select {
				case <-ctx.Done():
					timer.Stop()
					h.closeAll()
					return
				case <-timer.C:
				}
			case <-safety.C:
			}
			h.read()
		}
	}()
}

// read compares every pipeline's fingerprint with the last read and sends
// the pipelines that differ to every client.
func (h *focusEventHub) read() {
	h.mu.Lock()
	if len(h.subs) == 0 {
		h.last = nil
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	next, ok := h.fingerprints()
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last != nil {
		for _, id := range focusEventChanges(h.last, next) {
			h.sendLocked(focusEvent{Type: "pipeline", SessionID: id})
		}
	}
	h.last = next
}

// fingerprints reads every pipeline's fingerprint now; ok is false when
// the pipelines cannot be read.
func (h *focusEventHub) fingerprints() (map[string]string, bool) {
	if h.pipelines == nil {
		return nil, false
	}
	pipelines, err := h.pipelines.List()
	if err != nil {
		return nil, false
	}
	h.mu.Lock()
	activity := h.activity
	h.mu.Unlock()
	snap := chatActivitySnapshot{}
	if activity != nil {
		snap = activity()
	}
	var queued []chatQueuedApproval
	if h.queued != nil {
		queued, _ = h.queued()
	}
	return focusEventFingerprints(pipelines, snap, queued), true
}

// subscribe adds a client. The returned func removes it. With no client
// before it, it reads the baseline the next change is compared with before
// it returns, so a change right after the client hears "connected" is
// never lost.
func (h *focusEventHub) subscribe() (<-chan focusEvent, func()) {
	ch := make(chan focusEvent, focusEventsBuffer)
	h.mu.Lock()
	id := h.nextSub
	h.nextSub++
	h.subs[id] = ch
	needBaseline := h.last == nil
	h.mu.Unlock()
	if needBaseline {
		if base, ok := h.fingerprints(); ok {
			h.mu.Lock()
			// A read that finished meanwhile is newer: keep it.
			if h.last == nil {
				h.last = base
			}
			h.mu.Unlock()
		}
	}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
	}
}

// sendLocked sends evt to every client; a client whose buffer is full is
// dropped (its stream ends, and it reads everything when it reconnects).
func (h *focusEventHub) sendLocked(evt focusEvent) {
	for id, ch := range h.subs {
		select {
		case ch <- evt:
		default:
			delete(h.subs, id)
			close(ch)
		}
	}
}

func (h *focusEventHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
}

// focusEventFingerprints is one fingerprint per pipeline, by its session:
// the pipeline itself, and the running turns, questions and queued tool
// calls of its session and its Q&A session.
func focusEventFingerprints(pipelines []focuspipeline.Pipeline, snap chatActivitySnapshot, queued []chatQueuedApproval) map[string]string {
	running := map[string]bool{}
	for _, r := range snap.Running {
		running[r.SessionID] = true
	}
	asks := map[string][]string{}
	for _, p := range snap.Pending {
		asks[p.SessionID] = append(asks[p.SessionID], "ask:"+p.RequestID)
	}
	for _, q := range queued {
		asks[q.SessionID] = append(asks[q.SessionID], "queued:"+q.ApprovalID)
	}
	out := make(map[string]string, len(pipelines))
	for _, p := range pipelines {
		id := strings.TrimSpace(p.SessionID)
		if id == "" {
			continue
		}
		hash := fnv.New64a()
		raw, _ := json.Marshal(p)
		_, _ = hash.Write(raw)
		for _, sessionID := range []string{id, strings.TrimSpace(p.QASessionID)} {
			if sessionID == "" {
				continue
			}
			list := slices.Clone(asks[sessionID])
			slices.Sort(list)
			_, _ = fmt.Fprintf(hash, "\n%s running=%t %s", sessionID, running[sessionID], strings.Join(list, ","))
		}
		out[id] = fmt.Sprintf("%016x", hash.Sum64())
	}
	return out
}

// focusEventChanges lists the pipelines whose fingerprint differs between
// prev and next, including ones added or removed, in a stable order.
func focusEventChanges(prev, next map[string]string) []string {
	var out []string
	for id, fp := range next {
		if prev[id] != fp {
			out = append(out, id)
		}
	}
	for id := range prev {
		if _, ok := next[id]; !ok {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// newFocusEventsHandler serves GET /v1/focus/events as server-sent events:
// "connected" first, then "pipeline" events, with keepalives between.
func newFocusEventsHandler(hub *focusEventHub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		if hub == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "focus events are not configured")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "", "streaming is not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		events, unsubscribe := hub.subscribe()
		defer unsubscribe()
		write := func(evt focusEvent) bool {
			raw, _ := json.Marshal(evt)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		if !write(focusEvent{Type: "connected"}) {
			return
		}
		keepalive := time.NewTicker(focusEventsKeepalive)
		defer keepalive.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-keepalive.C:
				if !write(focusEvent{Type: "keepalive"}) {
					return
				}
			case evt, open := <-events:
				if !open || !write(evt) {
					return
				}
			}
		}
	})
}
