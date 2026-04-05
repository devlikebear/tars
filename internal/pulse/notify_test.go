package pulse

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type captureNotifier struct {
	mu     sync.Mutex
	events []NotifyEvent
	err    error
}

func (c *captureNotifier) Notify(ctx context.Context, e NotifyEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.events = append(c.events, e)
	return nil
}

func (c *captureNotifier) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestNotifyRouter_RoutesNotifyDecision(t *testing.T) {
	cap := &captureNotifier{}
	r := NewNotifyRouter(cap, NotifyConfig{MinSeverity: SeverityWarn})
	delivered, err := r.Route(context.Background(), Decision{
		Action:   ActionNotify,
		Severity: SeverityError,
		Title:    "disk",
		Summary:  "usage 95%",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !delivered {
		t.Error("expected delivered=true")
	}
	if cap.len() != 1 {
		t.Fatalf("events captured = %d, want 1", cap.len())
	}
	got := cap.events[0]
	if got.Category != "pulse" || got.Title != "disk" || got.Severity != SeverityError {
		t.Errorf("unexpected event: %+v", got)
	}
}

func TestNotifyRouter_DropsBelowMinSeverity(t *testing.T) {
	cap := &captureNotifier{}
	r := NewNotifyRouter(cap, NotifyConfig{MinSeverity: SeverityWarn})
	delivered, _ := r.Route(context.Background(), Decision{
		Action:   ActionNotify,
		Severity: SeverityInfo,
		Title:    "x",
	})
	if delivered {
		t.Error("expected delivered=false for info below warn floor")
	}
	if cap.len() != 0 {
		t.Errorf("events = %d, want 0", cap.len())
	}
}

func TestNotifyRouter_IgnoresNonNotifyActions(t *testing.T) {
	cap := &captureNotifier{}
	r := NewNotifyRouter(cap, NotifyConfig{})
	for _, action := range []Action{ActionIgnore, ActionAutofix} {
		delivered, _ := r.Route(context.Background(), Decision{
			Action:   action,
			Severity: SeverityCritical,
			Title:    "x",
		})
		if delivered {
			t.Errorf("action %q should not be routed", action)
		}
	}
	if cap.len() != 0 {
		t.Errorf("events = %d, want 0", cap.len())
	}
}

func TestNotifyRouter_NilNotifierIsNoop(t *testing.T) {
	r := NewNotifyRouter(nil, NotifyConfig{})
	delivered, err := r.Route(context.Background(), Decision{
		Action:   ActionNotify,
		Severity: SeverityCritical,
		Title:    "x",
	})
	if err != nil || delivered {
		t.Errorf("nil notifier: delivered=%v err=%v", delivered, err)
	}
}

func TestNotifyRouter_NilReceiverIsNoop(t *testing.T) {
	var r *NotifyRouter
	delivered, err := r.Route(context.Background(), Decision{Action: ActionNotify, Severity: SeverityCritical, Title: "x"})
	if err != nil || delivered {
		t.Errorf("nil router: delivered=%v err=%v", delivered, err)
	}
}

func TestNotifyRouter_PropagatesNotifierError(t *testing.T) {
	cap := &captureNotifier{err: errors.New("broker down")}
	r := NewNotifyRouter(cap, NotifyConfig{})
	delivered, err := r.Route(context.Background(), Decision{
		Action:   ActionNotify,
		Severity: SeverityWarn,
		Title:    "x",
	})
	if delivered {
		t.Error("delivered should be false on error")
	}
	if err == nil {
		t.Error("expected error")
	}
}

func TestNotifyRouter_EventTimestampSet(t *testing.T) {
	cap := &captureNotifier{}
	r := NewNotifyRouter(cap, NotifyConfig{})
	fixedTime := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return fixedTime }
	_, _ = r.Route(context.Background(), Decision{Action: ActionNotify, Severity: SeverityWarn, Title: "x"})
	if cap.events[0].Timestamp != fixedTime {
		t.Errorf("timestamp = %v, want %v", cap.events[0].Timestamp, fixedTime)
	}
}

func TestNotifierFunc(t *testing.T) {
	called := false
	var fn Notifier = NotifierFunc(func(ctx context.Context, e NotifyEvent) error {
		called = true
		if e.Title != "hello" {
			t.Errorf("title = %q", e.Title)
		}
		return nil
	})
	if err := fn.Notify(context.Background(), NotifyEvent{Title: "hello"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !called {
		t.Error("func not called")
	}
}
