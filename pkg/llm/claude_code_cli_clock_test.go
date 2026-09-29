package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestClaudeCodeTurnClockExpires(t *testing.T) {
	ctx, clock := newClaudeCodeTurnClock(context.Background(), 30*time.Millisecond)
	defer clock.stop()

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the turn outlived its timeout")
	}
	if !errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		t.Fatalf("cause = %v, want DeadlineExceeded so the turn reports a timeout", context.Cause(ctx))
	}
}

// Time a permission prompt spends waiting on a person does not count: the
// CLI itself waits indefinitely (150s observed on 2.1.283), and a person may
// well take longer than the whole turn's budget.
func TestClaudeCodeTurnClockDoesNotCountPromptWaits(t *testing.T) {
	ctx, clock := newClaudeCodeTurnClock(context.Background(), 60*time.Millisecond)
	defer clock.stop()

	handler := clock.untimed(func(context.Context, ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		time.Sleep(200 * time.Millisecond)
		return ClaudeCodePermissionDecision{Allow: true}, nil
	})
	if _, err := handler(ctx, ClaudeCodePermissionRequest{}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("the prompt wait was charged to the turn: %v", context.Cause(ctx))
	}
	// The clock runs again afterwards with what was left.
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock never resumed")
	}
}

// Two prompts can be open at once (a subagent's and the parent's); the clock
// stays stopped until both are answered.
func TestClaudeCodeTurnClockOverlappingPrompts(t *testing.T) {
	ctx, clock := newClaudeCodeTurnClock(context.Background(), 60*time.Millisecond)
	defer clock.stop()

	release := make(chan struct{})
	handler := clock.untimed(func(context.Context, ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		<-release
		return ClaudeCodePermissionDecision{}, nil
	})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = handler(ctx, ClaudeCodePermissionRequest{})
		}()
	}
	time.Sleep(150 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatalf("expired while prompts were open: %v", context.Cause(ctx))
	}
	close(release)
	wg.Wait()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock never resumed after both prompts closed")
	}
}

func TestClaudeCodeTurnClockFollowsParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, clock := newClaudeCodeTurnClock(parent, time.Hour)
	defer clock.stop()
	cancel()
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.Canceled) || errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		t.Fatalf("a caller cancel must not read as a timeout: err=%v cause=%v", ctx.Err(), context.Cause(ctx))
	}
}
