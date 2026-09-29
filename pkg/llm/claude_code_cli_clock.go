package llm

import (
	"context"
	"sync"
	"time"
)

// claudeCodeTurnClock bounds one claude invocation like context.WithTimeout,
// except that it stops while a permission prompt waits on a person. Claude
// Code itself waits for an answer indefinitely, so a person taking longer
// than CLAUDE_CODE_CLI_TIMEOUT to decide would otherwise fail the whole turn.
//
// Expiry cancels the context with cause context.DeadlineExceeded, which is
// how the turn tells a timeout from a caller cancel.
type claudeCodeTurnClock struct {
	cancel context.CancelCauseFunc

	mu        sync.Mutex
	timer     *time.Timer
	remaining time.Duration
	started   time.Time
	waiting   int
	stopped   bool
}

func newClaudeCodeTurnClock(parent context.Context, timeout time.Duration) (context.Context, *claudeCodeTurnClock) {
	ctx, cancel := context.WithCancelCause(parent)
	clock := &claudeCodeTurnClock{cancel: cancel, remaining: timeout}
	clock.mu.Lock()
	clock.run()
	clock.mu.Unlock()
	return ctx, clock
}

// run starts the timer for the remaining budget. Callers hold mu.
func (c *claudeCodeTurnClock) run() {
	c.started = time.Now()
	c.timer = time.AfterFunc(c.remaining, func() { c.cancel(context.DeadlineExceeded) })
}

// untimed wraps handler so the clock stands still while it runs. Prompts may
// overlap (a subagent's and the parent's), so the clock restarts only when
// the last open one returns.
func (c *claudeCodeTurnClock) untimed(handler ClaudeCodePermissionHandler) ClaudeCodePermissionHandler {
	return func(ctx context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		c.hold()
		defer c.release()
		return handler(ctx, req)
	}
}

func (c *claudeCodeTurnClock) hold() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waiting++
	if c.waiting > 1 || c.stopped {
		return
	}
	// A timer that already fired has cancelled the turn; Stop returning
	// false leaves nothing to pause.
	if c.timer.Stop() {
		c.remaining -= time.Since(c.started)
	}
}

func (c *claudeCodeTurnClock) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waiting--
	if c.waiting > 0 || c.stopped {
		return
	}
	if c.remaining <= 0 {
		c.cancel(context.DeadlineExceeded)
		return
	}
	c.run()
}

// stop releases the timer and the context. Call it when the turn ends.
func (c *claudeCodeTurnClock) stop() {
	c.mu.Lock()
	c.stopped = true
	c.timer.Stop()
	c.mu.Unlock()
	c.cancel(context.Canceled)
}
