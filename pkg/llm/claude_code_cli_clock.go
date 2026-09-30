package llm

import (
	"context"
	"sync"
	"time"
)

// claudeCodeTurnClock bounds how long one claude invocation may go silent.
// Every line the CLI prints (touch) starts the window over, so a long coding
// turn that keeps producing output is never cut off; only a CLI that prints
// nothing for CLAUDE_CODE_CLI_TIMEOUT is.
//
// The clock also stands still while a permission prompt waits on a person.
// Claude Code itself waits for an answer indefinitely, so a person taking
// longer than the limit to decide would otherwise fail the whole turn. The
// answer counts as activity: the window starts over when the last prompt
// closes.
//
// Expiry cancels the context with cause context.DeadlineExceeded, which is
// how the turn tells a timeout from a caller cancel.
type claudeCodeTurnClock struct {
	cancel context.CancelCauseFunc
	idle   time.Duration

	mu      sync.Mutex
	timer   *time.Timer
	waiting int
	stopped bool
	expired bool
}

func newClaudeCodeTurnClock(parent context.Context, idle time.Duration) (context.Context, *claudeCodeTurnClock) {
	ctx, cancel := context.WithCancelCause(parent)
	clock := &claudeCodeTurnClock{cancel: cancel, idle: idle}
	clock.mu.Lock()
	clock.timer = time.AfterFunc(idle, clock.expire)
	clock.mu.Unlock()
	return ctx, clock
}

func (c *claudeCodeTurnClock) expire() {
	c.mu.Lock()
	c.expired = true
	c.mu.Unlock()
	c.cancel(context.DeadlineExceeded)
}

// restart gives the turn a fresh idle window. Callers hold mu. A timer that
// already fired has cancelled the turn and is not revived.
func (c *claudeCodeTurnClock) restart() {
	if c.expired {
		return
	}
	c.timer.Reset(c.idle)
}

// touch records a sign of life from the CLI. While a prompt is open the
// clock stays stopped; release starts it again.
func (c *claudeCodeTurnClock) touch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.waiting > 0 || c.stopped {
		return
	}
	c.restart()
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
	c.timer.Stop()
}

func (c *claudeCodeTurnClock) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waiting--
	if c.waiting > 0 || c.stopped {
		return
	}
	c.restart()
}

// stop releases the timer and the context. Call it when the turn ends.
func (c *claudeCodeTurnClock) stop() {
	c.mu.Lock()
	c.stopped = true
	c.timer.Stop()
	c.mu.Unlock()
	c.cancel(context.Canceled)
}
