package tarsserver

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Background streams (#971). A chat turn no longer ends when the console
// that started it goes away: its context is detached from the request, so
// switching sessions, reloading, or a dropped connection leaves the turn
// running, and only POST /v1/chat/cancel stops it. Every event the turn
// streams is also kept in the session's turn feed, and
// GET /v1/chat/stream?session_id= replays the feed and follows it, so a
// console coming back to the session picks the turn up where it is.

// chatTurnFeedMaxEvents bounds a feed. A turn that streams more drops its
// oldest events, and a console attaching then reloads the history instead
// of rebuilding the turn from them (turn_feed_truncated).
const chatTurnFeedMaxEvents = 20000

// chatTurnFeed is one running turn's events.
type chatTurnFeed struct {
	mu      sync.Mutex
	events  [][]byte
	dropped int
	done    bool
	// changed is closed and replaced whenever an event arrives or the turn
	// ends, waking followers.
	changed chan struct{}
}

func newChatTurnFeed() *chatTurnFeed {
	return &chatTurnFeed{changed: make(chan struct{})}
}

func (f *chatTurnFeed) publish(event []byte) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	if len(f.events) >= chatTurnFeedMaxEvents {
		f.events = f.events[1:]
		f.dropped++
	}
	f.events = append(f.events, append([]byte(nil), event...))
	close(f.changed)
	f.changed = make(chan struct{})
}

func (f *chatTurnFeed) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	close(f.changed)
}

// since returns the events from absolute position next on, the position
// after them, whether the turn is over, and a channel closed on the next
// change. Positions count dropped events, so a follower never re-reads one.
func (f *chatTurnFeed) since(next int) ([][]byte, int, bool, bool, <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	truncated := next < f.dropped
	start := next - f.dropped
	if start < 0 {
		start = 0
	}
	if start > len(f.events) {
		start = len(f.events)
	}
	events := f.events[start:]
	return events, f.dropped + len(f.events), f.done, truncated, f.changed
}

// chatTurnFeeds holds the feed of each session's running turn.
type chatTurnFeeds struct {
	mu    sync.Mutex
	feeds map[string]*chatTurnFeed
}

func newChatTurnFeeds() *chatTurnFeeds {
	return &chatTurnFeeds{feeds: map[string]*chatTurnFeed{}}
}

// begin starts a feed for a turn in sessionID and returns it with the func
// that ends it.
func (r *chatTurnFeeds) begin(sessionID string) (*chatTurnFeed, func()) {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return nil, func() {}
	}
	feed := newChatTurnFeed()
	r.mu.Lock()
	r.feeds[sessionID] = feed
	r.mu.Unlock()
	return feed, func() {
		feed.finish()
		r.mu.Lock()
		if r.feeds[sessionID] == feed {
			delete(r.feeds, sessionID)
		}
		r.mu.Unlock()
	}
}

func (r *chatTurnFeeds) get(sessionID string) *chatTurnFeed {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.feeds[sessionID]
}

// chatTurnKeepAlive is how often an idle attached stream sends a comment
// line, so proxies do not close it while a turn waits on a tool or a
// person.
const chatTurnKeepAlive = 15 * time.Second

// handleChatTurnStream serves GET /v1/chat/stream?session_id=: 204 when no
// turn is running, otherwise the turn's events so far and then the rest as
// they come, in the same SSE form as POST /v1/chat.
func handleChatTurnStream(w http.ResponseWriter, r *http.Request, feeds *chatTurnFeeds) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "session_id is required")
		return
	}
	feed := feeds.get(sessionID)
	if feed == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	next := 0
	keepAlive := time.NewTicker(chatTurnKeepAlive)
	defer keepAlive.Stop()
	for {
		events, after, done, truncated, changed := feed.since(next)
		if truncated && next == 0 {
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"turn_feed_truncated\",\"session_id\":%q}\n\n", sessionID)
		}
		for _, event := range events {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", event); err != nil {
				return
			}
		}
		next = after
		flush()
		if done {
			return
		}
		select {
		case <-changed:
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flush()
		case <-r.Context().Done():
			return
		}
	}
}
