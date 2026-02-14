package agent

import (
	"context"
	"sync"
	"time"
)

type CounterHook struct {
	mu     sync.Mutex
	counts map[EventType]int
}

func NewCounterHook() *CounterHook {
	return &CounterHook{
		counts: map[EventType]int{},
	}
}

func (h *CounterHook) OnEvent(_ context.Context, evt Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts[evt.Type]++
}

func (h *CounterHook) Snapshot() map[EventType]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[EventType]int, len(h.counts))
	for k, v := range h.counts {
		out[k] = v
	}
	return out
}

type AuditEntry struct {
	Time         time.Time `json:"time"`
	Type         EventType `json:"type"`
	Iteration    int       `json:"iteration,omitempty"`
	MessageCount int       `json:"message_count,omitempty"`
	ToolName     string    `json:"tool_name,omitempty"`
	ToolCallID   string    `json:"tool_call_id,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type AuditHook struct {
	mu      sync.Mutex
	max     int
	entries []AuditEntry
}

func NewAuditHook(maxEntries int) *AuditHook {
	return &AuditHook{
		max:     maxEntries,
		entries: make([]AuditEntry, 0, maxEntries),
	}
}

func (h *AuditHook) OnEvent(_ context.Context, evt Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := AuditEntry{
		Time:         time.Now().UTC(),
		Type:         evt.Type,
		Iteration:    evt.Iteration,
		MessageCount: evt.MessageCount,
		ToolName:     evt.ToolName,
		ToolCallID:   evt.ToolCallID,
	}
	if evt.Err != nil {
		entry.Error = evt.Err.Error()
	}

	h.entries = append(h.entries, entry)
	if h.max > 0 && len(h.entries) > h.max {
		h.entries = append([]AuditEntry(nil), h.entries[len(h.entries)-h.max:]...)
	}
}

func (h *AuditHook) Entries() []AuditEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]AuditEntry, len(h.entries))
	copy(out, h.entries)
	return out
}
