package initiative

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// liveReplyRuntime builds a live-mode runtime with a fakeSpeaker that
// always delivers, and an optional text-signal backend (one, nil means no
// backend — quiet_requested then always reads false) for reply tracking's
// end-to-end tests (tars#1221): these drive resolveRepliesLocked,
// applyOutcomeLocked and Snapshot's shared aggregation through a real
// Runtime, unlike reply_test.go's direct unit tests of the pure functions.
func liveReplyRuntime(t *testing.T, now *time.Time, one SystemOne, includeText bool) (*Runtime, *fakeObserver) {
	t.Helper()
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	observer := &fakeObserver{}
	deps := Dependencies{
		Observer:    observer,
		Ledger:      ledger,
		Logger:      zerolog.Nop(),
		Now:         func() time.Time { return *now },
		Speaker:     &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}},
		SystemOne:   one,
		IncludeText: includeText,
	}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 12, DailyTextCalls: 60}, deps)
	return r, observer
}

// TestReplyTrackingRepliedWithinWindow covers the plain case: a
// main-session reply arrives within the reply window and the (unconfigured)
// text backend defaults quiet_requested to false, so the outcome is
// "replied".
func TestReplyTrackingRepliedWithinWindow(t *testing.T) {
	now := at(9, 0)
	r, observer := liveReplyRuntime(t, &now, nil, false)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	e1 := r.RunOnceAt(context.Background(), now)
	if e1.Intent != IntentGreet || e1.Delivery != DeliveryDelivered {
		t.Fatalf("tick1 = %+v, want a delivered greet", e1)
	}

	now = now.Add(5 * time.Minute)
	observer.obs = Observation{ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At, MainUserAt: now, LastUserAt: now}
	r.RunOnceAt(context.Background(), now)

	snap := r.Snapshot()
	if snap.Outcomes.Today.Replied != 1 || snap.Outcomes.ConsecutiveIgnored != 0 {
		t.Fatalf("outcomes = %+v, want 1 replied and a reset streak", snap.Outcomes)
	}
}

// TestReplyTrackingIgnoredAfterWindowElapses covers the no-reply case: the
// reply window elapses with no main-session message, so the outcome is
// "ignored" and the cooldown now in force doubles.
func TestReplyTrackingIgnoredAfterWindowElapses(t *testing.T) {
	now := at(9, 0)
	r, observer := liveReplyRuntime(t, &now, nil, false)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	e1 := r.RunOnceAt(context.Background(), now)
	if e1.Delivery != DeliveryDelivered {
		t.Fatalf("tick1 = %+v, want a delivered speak", e1)
	}

	now = now.Add(30 * time.Minute) // the default reply window (30m) elapses
	observer.obs = Observation{ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At}
	r.RunOnceAt(context.Background(), now)

	snap := r.Snapshot()
	if snap.Outcomes.Today.Ignored != 1 || snap.Outcomes.ConsecutiveIgnored != 1 {
		t.Fatalf("outcomes = %+v, want 1 ignored and a streak of 1", snap.Outcomes)
	}
	if want := EffectiveCooldown(45*time.Minute, 1).Seconds(); snap.Outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("EffectiveCooldownSeconds = %v, want %v (doubled)", snap.Outcomes.EffectiveCooldownSeconds, want)
	}
}

// TestReplyTrackingStoppedBlocksRestOfDay covers the quiet-request case: a
// reply within the window that the text backend reads as quiet_requested
// resolves to "stopped", which then blocks every Speak decision for the
// rest of that local day (not just the quiet_requested tick itself).
func TestReplyTrackingStoppedBlocksRestOfDay(t *testing.T) {
	now := at(9, 0)
	fake := &fakeSystemOne{probs: map[string]float64{"quiet_requested": 0.9}}
	r, observer := liveReplyRuntime(t, &now, fake, true)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	e1 := r.RunOnceAt(context.Background(), now)
	if e1.Delivery != DeliveryDelivered {
		t.Fatalf("tick1 = %+v, want a delivered speak", e1)
	}

	now = now.Add(5 * time.Minute)
	observer.obs = Observation{
		ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At, MainUserAt: now, LastUserAt: now,
		RecentUser: []UserMessage{{At: now, Text: "조용히 해줘"}},
	}
	r.RunOnceAt(context.Background(), now)

	snap := r.Snapshot()
	if snap.Outcomes.Today.Stopped != 1 || !snap.Outcomes.StoppedToday {
		t.Fatalf("outcomes = %+v, want 1 stopped and StoppedToday", snap.Outcomes)
	}

	// Later the same local day, even a fresh arrival must stay silent:
	// stopped_today outranks the arrival greet, and no text backend call is
	// needed to reach that decision (no pending reply, and every text
	// combination already yields the same Intent/Speak).
	now = now.Add(6 * time.Hour)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	e3 := r.RunOnceAt(context.Background(), now)
	if e3.Intent != IntentNone || e3.Reason != "stopped_today" || e3.Called {
		t.Fatalf("tick3 = %+v, want a silent stopped_today decision with no backend call", e3)
	}
}

// TestReplyTrackingWaitsThroughTextReadBackoffThenResolves is the review
// follow-up's end-to-end guard (tars#1221): a reply arrives while the text
// backend is erroring, stays pending across ticks the existing
// read-failure backoff keeps from calling the backend at all, and only
// resolves once a later tick's read actually succeeds — proving the
// pending reply does not cause the backend to be called every tick
// (reuses the same backoff schedule as the text-signal read generally).
func TestReplyTrackingWaitsThroughTextReadBackoffThenResolves(t *testing.T) {
	now := at(9, 0)
	fake := &fakeSystemOne{err: errors.New("backend down")}
	r, observer := liveReplyRuntime(t, &now, fake, true)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	e1 := r.RunOnceAt(context.Background(), now)
	if e1.Delivery != DeliveryDelivered {
		t.Fatalf("tick1 = %+v, want a delivered speak", e1)
	}

	replyAt := now.Add(5 * time.Minute)
	recentUser := []UserMessage{{At: replyAt, Text: "whatever"}}

	// The reply arrives, but this tick's read errors: the outcome must
	// not be finalized yet.
	now = replyAt
	observer.obs = Observation{ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At, MainUserAt: replyAt, LastUserAt: replyAt, RecentUser: recentUser}
	r.RunOnceAt(context.Background(), now)
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want exactly one attempt", fake.calls)
	}
	if snap := r.Snapshot(); snap.Outcomes.Today.Replied != 0 || snap.Outcomes.Today.Stopped != 0 {
		t.Fatalf("outcomes = %+v, want nothing finalized yet", snap.Outcomes)
	}

	// One minute later is still inside the 5m read-failure backoff: the
	// backend must not be called again, and the reply stays pending.
	now = replyAt.Add(time.Minute)
	observer.obs = Observation{ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At, MainUserAt: replyAt, LastUserAt: replyAt, RecentUser: recentUser}
	r.RunOnceAt(context.Background(), now)
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want still exactly one attempt (backoff must hold)", fake.calls)
	}

	// Past the 5m backoff, the backend is reachable again.
	fake.err = nil
	fake.probs = map[string]float64{"quiet_requested": 0}
	now = replyAt.Add(6 * time.Minute)
	observer.obs = Observation{ConsolePresentAt: e1.At, ConsoleArrivedAt: e1.At, MainUserAt: replyAt, LastUserAt: replyAt, RecentUser: recentUser}
	r.RunOnceAt(context.Background(), now)
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want a retry once the backoff lifted", fake.calls)
	}
	if snap := r.Snapshot(); snap.Outcomes.Today.Replied != 1 {
		t.Fatalf("outcomes = %+v, want 1 replied once the read finally succeeded", snap.Outcomes)
	}
}
