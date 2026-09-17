package computeruse

import (
	"context"
	"errors"
)

// Effect is what an action turned out to do, as judged against a fresh
// observation rather than the driver's own success report.
type Effect string

const (
	EffectConfirmed      Effect = "confirmed"
	EffectPartial        Effect = "partial"
	EffectUnverifiable   Effect = "unverifiable"
	EffectSuspectedNoop  Effect = "suspected_noop"
	EffectRefused        Effect = "refused"
	EffectDeliveryFailed Effect = "delivery_failed"
)

// Driver observes and drives a native GUI window. Implementations talk to a
// host automation backend; the loop never touches one directly.
type Driver interface {
	Ping(ctx context.Context) error                                // 바이너리+데몬 확인
	ResolveWindow(ctx context.Context, app string) (Window, error) // app=="" → 최전면
	Snapshot(ctx context.Context, w Window, opts SnapshotOpts) (Snapshot, error)
	Click(ctx context.Context, w Window, token string) (Effect, error)
	SetValue(ctx context.Context, w Window, token, value string) (Effect, error)
	TypeText(ctx context.Context, w Window, token, text string) (Effect, error)
	PressKey(ctx context.Context, w Window, key string) (Effect, error)
	Scroll(ctx context.Context, w Window, direction string) (Effect, error) // "up"|"down"
}

// ErrDriverUnavailable reports that no driver backend is usable — the binary
// is missing, the daemon is down, or permissions were refused.
var ErrDriverUnavailable = errors.New("computeruse: driver unavailable")
