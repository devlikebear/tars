package tarsserver

import (
	"context"
	"sync"
)

// chatCancelRegistry tracks active chat sessions and allows cancellation.
type chatCancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newChatCancelRegistry() *chatCancelRegistry {
	return &chatCancelRegistry{
		cancels: make(map[string]context.CancelFunc),
	}
}

// Register stores the cancel function for a session.
func (r *chatCancelRegistry) Register(sessionID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[sessionID] = cancel
}

// Cancel invokes the cancel function for a session and removes it.
// Returns true if a cancel was found and invoked.
func (r *chatCancelRegistry) Cancel(sessionID string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[sessionID]
	if ok {
		delete(r.cancels, sessionID)
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// Unregister removes the cancel function without invoking it.
func (r *chatCancelRegistry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, sessionID)
}
