package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// chatCancelWait bounds how long POST /v1/chat/cancel waits for the
// cancelled turn to let go of its session, like focusPreemptWait for a run.
const chatCancelWait = 30 * time.Second

// errChatTurnBusy is a turn that could not start because another turn —
// a person's or the focus driver's — holds the session.
var errChatTurnBusy = errors.New("a turn is already running on this session")

// chatCancelRegistry holds each session's running turn: one claim per
// session at a time, taken before the turn is prepared, so two turns never
// run on one session (their feeds and cancels would clobber each other).
type chatCancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]*chatCancelEntry
}

// chatCancelEntry is one claim. Its cancel func arrives once the turn has a
// context; a cancel before that is remembered and fired then. ended closes
// when the claim is released.
type chatCancelEntry struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	cancelled bool
	ended     chan struct{}
	endOnce   sync.Once
}

// setCancel gives the claim its turn's cancel func.
func (e *chatCancelEntry) setCancel(cancel context.CancelFunc) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.cancelled {
		e.mu.Unlock()
		cancel()
		return
	}
	e.cancel = cancel
	e.mu.Unlock()
}

func (e *chatCancelEntry) fire() {
	e.mu.Lock()
	e.cancelled = true
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func newChatCancelRegistry() *chatCancelRegistry {
	return &chatCancelRegistry{
		cancels: make(map[string]*chatCancelEntry),
	}
}

// Claim takes sessionID for one turn. ok is false when another turn holds
// it. release ends this claim (and only this one). An empty id claims
// nothing and always succeeds, as does a nil registry.
func (r *chatCancelRegistry) Claim(sessionID string) (*chatCancelEntry, func(), bool) {
	if r == nil || sessionID == "" {
		return nil, func() {}, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, busy := r.cancels[sessionID]; busy {
		return nil, func() {}, false
	}
	entry := &chatCancelEntry{ended: make(chan struct{})}
	r.cancels[sessionID] = entry
	return entry, func() {
		r.mu.Lock()
		if r.cancels[sessionID] == entry {
			delete(r.cancels, sessionID)
		}
		r.mu.Unlock()
		entry.endOnce.Do(func() { close(entry.ended) })
	}, true
}

// Cancel cancels the session's running turn. The claim stays until the
// turn ends and releases it, so no new turn starts beside one still
// winding down. It reports whether a turn was running, and returns a
// channel that closes when that turn has released the session.
func (r *chatCancelRegistry) Cancel(sessionID string) (<-chan struct{}, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	entry, ok := r.cancels[sessionID]
	r.mu.Unlock()
	if !ok {
		return nil, false
	}
	entry.fire()
	return entry.ended, true
}

// Running reports whether a turn holds the session.
func (r *chatCancelRegistry) Running(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.cancels[sessionID]
	return ok
}

// handleChatCancel is POST /v1/chat/cancel. It stops the session's turn and
// the focus driver's run, then answers once the turn has let go of the
// session (bounded by wait): a cancelled turn still writes its end
// checkpoint and transcript, and a client that sends its next message on
// the answer — a queued message resumed after Stop — would otherwise get a
// 409 for a turn that is already stopping. ended is false when the wait ran
// out first.
func handleChatCancel(w http.ResponseWriter, r *http.Request, registry *chatCancelRegistry, focus *focusDriver, wait time.Duration) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "", "session_id is required")
		return
	}
	// Cancel both: the running turn, and the focus driver's run that
	// would otherwise start the next one.
	turnEnded, turnCancelled := registry.Cancel(sessionID)
	if !focus.cancel(sessionID) && !turnCancelled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active chat for session"})
		return
	}
	ended := true
	if turnCancelled {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-turnEnded:
		case <-timer.C:
			ended = false
		case <-r.Context().Done():
			ended = false
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true, "ended": ended})
}
