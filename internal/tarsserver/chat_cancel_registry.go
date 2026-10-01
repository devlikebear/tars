package tarsserver

import (
	"context"
	"errors"
	"sync"
)

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
// context; a cancel before that is remembered and fired then.
type chatCancelEntry struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	cancelled bool
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
	entry := &chatCancelEntry{}
	r.cancels[sessionID] = entry
	return entry, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.cancels[sessionID] == entry {
			delete(r.cancels, sessionID)
		}
	}, true
}

// Cancel cancels the session's running turn. The claim stays until the
// turn ends and releases it, so no new turn starts beside one still
// winding down. It reports whether a turn was running.
func (r *chatCancelRegistry) Cancel(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	entry, ok := r.cancels[sessionID]
	r.mu.Unlock()
	if ok {
		entry.fire()
	}
	return ok
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
