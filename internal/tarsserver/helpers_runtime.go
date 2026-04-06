package tarsserver

import (
	"context"
	"sync/atomic"
)

// runtimeActivity tracks lightweight server-wide activity counters used
// by pipeline code to decide whether to proceed with background work.
// Previously used by the heartbeat runtime to suppress ticks during
// active chat; retained for the chat pipeline's own gating needs.
type runtimeActivity struct {
	chatInFlight atomic.Int64
}

type gatewayPromptRunner func(ctx context.Context, runLabel string, promptText string, allowedTools []string, tier string) (string, error)

func (a *runtimeActivity) beginChat() func() {
	if a == nil {
		return func() {}
	}
	a.chatInFlight.Add(1)
	return func() {
		a.chatInFlight.Add(-1)
	}
}

func (a *runtimeActivity) isChatBusy() bool {
	if a == nil {
		return false
	}
	return a.chatInFlight.Load() > 0
}
