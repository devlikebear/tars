package tarsserver

import (
	"context"
	"sync"
)

// chatCancelRegistry tracks active chat sessions and allows cancellation.
type chatCancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]*chatCancelEntry
}

// chatCancelEntry is one registration; ending it only removes that same
// registration, never a later one of the same session.
type chatCancelEntry struct{ cancel context.CancelFunc }

func newChatCancelRegistry() *chatCancelRegistry {
	return &chatCancelRegistry{
		cancels: make(map[string]*chatCancelEntry),
	}
}

// Register stores the cancel function for a session and returns the func
// that removes this registration (and only this one) without invoking it.
func (r *chatCancelRegistry) Register(sessionID string, cancel context.CancelFunc) func() {
	if r == nil {
		return func() {}
	}
	entry := &chatCancelEntry{cancel: cancel}
	r.mu.Lock()
	r.cancels[sessionID] = entry
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.cancels[sessionID] == entry {
			delete(r.cancels, sessionID)
		}
	}
}

// Cancel invokes the cancel function for a session and removes it.
// Returns true if a cancel was found and invoked.
func (r *chatCancelRegistry) Cancel(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	entry, ok := r.cancels[sessionID]
	if ok {
		delete(r.cancels, sessionID)
	}
	r.mu.Unlock()
	if ok {
		entry.cancel()
	}
	return ok
}

// Running reports whether a turn is registered for the session.
func (r *chatCancelRegistry) Running(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.cancels[sessionID]
	return ok
}
