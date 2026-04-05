package pulse

import (
	"context"
	"time"
)

// NotifyEvent is a transport-neutral notification payload emitted by
// pulse. The Notifier implementation decides where to fan it out
// (session event stream, telegram, etc.).
//
// Keep this struct intentionally small: pulse does not know about
// delivery channels, user sessions, or chat IDs. It simply asks "please
// tell the user about this".
type NotifyEvent struct {
	Category  string         `json:"category"`
	Severity  Severity       `json:"severity"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// Notifier is the sink pulse emits NotifyEvents to. The real
// implementation lives in internal/tarsserver where it has access to the
// event broker and telegram sender. Tests use a tiny fake.
//
// Notify must not block for long and must not return an error that would
// abort the tick — the runtime records failure in state but proceeds.
type Notifier interface {
	Notify(ctx context.Context, event NotifyEvent) error
}

// NotifierFunc adapts a plain function to the Notifier interface.
type NotifierFunc func(ctx context.Context, event NotifyEvent) error

func (f NotifierFunc) Notify(ctx context.Context, event NotifyEvent) error {
	return f(ctx, event)
}

// NotifyConfig controls the pulse→notifier filter. Decisions whose
// severity falls below MinSeverity are dropped entirely so operators can
// run pulse with a high threshold without spamming the UI.
type NotifyConfig struct {
	MinSeverity Severity
}

// NotifyRouter converts Decisions to NotifyEvents and forwards them to a
// Notifier, honoring the minimum-severity floor. It is stateless aside
// from its configured dependencies.
type NotifyRouter struct {
	notifier Notifier
	cfg      NotifyConfig
	now      func() time.Time
}

// NewNotifyRouter constructs a router. A nil notifier is allowed — all
// calls become no-ops, which is useful for tests and for configurations
// where the user has disabled pulse notifications.
func NewNotifyRouter(notifier Notifier, cfg NotifyConfig) *NotifyRouter {
	return &NotifyRouter{
		notifier: notifier,
		cfg:      cfg,
		now:      time.Now,
	}
}

// Route delivers a decision to the notifier if its severity meets the
// configured floor and its action is ActionNotify. Returns (delivered,
// err). err is non-nil only when the notifier itself failed.
func (r *NotifyRouter) Route(ctx context.Context, decision Decision) (bool, error) {
	if r == nil || r.notifier == nil {
		return false, nil
	}
	if decision.Action != ActionNotify {
		return false, nil
	}
	if !decision.Severity.AtLeast(r.cfg.MinSeverity) {
		return false, nil
	}
	event := NotifyEvent{
		Category:  "pulse",
		Severity:  decision.Severity,
		Title:     decision.Title,
		Message:   decision.Summary,
		Details:   decision.Details,
		Timestamp: r.now(),
	}
	if err := r.notifier.Notify(ctx, event); err != nil {
		return false, err
	}
	return true, nil
}
