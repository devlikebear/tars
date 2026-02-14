package agent

import (
	"context"
	"errors"
	"testing"
)

func TestCounterHook(t *testing.T) {
	h := NewCounterHook()
	ctx := context.Background()

	h.OnEvent(ctx, Event{Type: EventLoopStart})
	h.OnEvent(ctx, Event{Type: EventBeforeLLM})
	h.OnEvent(ctx, Event{Type: EventBeforeLLM})
	h.OnEvent(ctx, Event{Type: EventLoopEnd})

	got := h.Snapshot()
	if got[EventLoopStart] != 1 {
		t.Fatalf("expected loop_start=1, got %d", got[EventLoopStart])
	}
	if got[EventBeforeLLM] != 2 {
		t.Fatalf("expected before_llm=2, got %d", got[EventBeforeLLM])
	}
	if got[EventLoopEnd] != 1 {
		t.Fatalf("expected loop_end=1, got %d", got[EventLoopEnd])
	}
}

func TestAuditHook_RespectsMaxEntries(t *testing.T) {
	h := NewAuditHook(2)
	ctx := context.Background()

	h.OnEvent(ctx, Event{Type: EventLoopStart})
	h.OnEvent(ctx, Event{Type: EventBeforeLLM, Iteration: 1})
	h.OnEvent(ctx, Event{Type: EventLoopError, Err: errors.New("boom")})

	entries := h.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != EventBeforeLLM {
		t.Fatalf("expected first retained entry before_llm, got %q", entries[0].Type)
	}
	if entries[1].Type != EventLoopError {
		t.Fatalf("expected last entry error, got %q", entries[1].Type)
	}
	if entries[1].Error == "" {
		t.Fatalf("expected error text to be captured")
	}
}
