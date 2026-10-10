package initiative

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// fakeSpeaker is a scriptable Speaker: each call to Speak pops the next
// outcome/error off outcomes (or repeats the last one once exhausted), and
// records every call for assertions.
type fakeSpeaker struct {
	mu       sync.Mutex
	outcomes []SpeakOutcome
	errs     []error
	calls    []SpeakRequest
	bodyOnly int
}

func (f *fakeSpeaker) Speak(_ context.Context, req SpeakRequest) (SpeakOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	idx := len(f.calls) - 1
	var outcome SpeakOutcome
	var err error
	if idx < len(f.outcomes) {
		outcome = f.outcomes[idx]
	} else if len(f.outcomes) > 0 {
		outcome = f.outcomes[len(f.outcomes)-1]
	}
	if idx < len(f.errs) {
		err = f.errs[idx]
	} else if len(f.errs) > 0 {
		err = f.errs[len(f.errs)-1]
	}
	return outcome, err
}

func (f *fakeSpeaker) NotifyBodyOnly(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodyOnly++
	return nil
}

func (f *fakeSpeaker) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func liveRuntime(t *testing.T, now *time.Time, speaker Speaker, ledger *Ledger) *Runtime {
	t.Helper()
	if ledger == nil {
		ledger = OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	}
	deps := Dependencies{
		Observer: &fakeObserver{}, Ledger: ledger, Logger: zerolog.Nop(),
		Now: func() time.Time { return *now }, Speaker: speaker,
	}
	return NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 12}, deps)
}

// TestLiveSpeakUnreachableWithoutConsoleNeverComposes checks requirement
// #2: a Speak intent with no console connected records
// Delivery=unreachable and never calls the composer at all.
func TestLiveSpeakUnreachableWithoutConsoleNeverComposes(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{}
	r := liveRuntime(t, &now, speaker, nil)
	// Reachable via Telegram (so Decide's long_absence check_in fires) but
	// the console itself is not connected (ConsoleConnectedAt zero): the
	// delivery channel is console-only, so this must still be unreachable.
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Intent != IntentCheckIn || !e.Speak {
		t.Fatalf("entry = %+v, want a check_in decision", e)
	}
	if e.Delivery != DeliveryUnreachable || e.Composed {
		t.Fatalf("entry delivery = %+v, want unreachable with no compose call", e)
	}
	if speaker.callCount() != 0 {
		t.Fatalf("speaker calls = %d, want 0", speaker.callCount())
	}
	// Cooldown/daily cap must not have advanced: a second tick with the
	// same observation must decide the same way, not "cooldown".
	now = now.Add(time.Minute)
	e2 := r.RunOnce(context.Background())
	if e2.Reason != "long_absence" || e2.Delivery != DeliveryUnreachable {
		t.Fatalf("second tick = %+v, want the same unreachable outcome (no cooldown)", e2)
	}
	// And the repeated identical unreachable tick is not written twice.
	recent, _ := r.deps.Ledger.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("ledger entries = %d, want 1 (repeated unreachable must not re-append)", len(recent))
	}
}

// TestLiveSpeakDeliveredWritesAssistantMessageAndCounts checks that a
// delivered speak counts toward cooldown/daily cap (requirement: "live에서
// delivered만 센다"), unlike unreachable above.
func TestLiveSpeakDeliveredWritesAssistantMessageAndCounts(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := liveRuntime(t, &now, speaker, nil)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryDelivered || !e.Composed {
		t.Fatalf("entry = %+v, want delivered", e)
	}
	if speaker.callCount() != 1 {
		t.Fatalf("speaker calls = %d, want 1", speaker.callCount())
	}
	now = now.Add(time.Minute)
	e2 := r.RunOnce(context.Background())
	if e2.Reason != "cooldown" {
		t.Fatalf("second tick = %+v, want cooldown after a delivered speak", e2)
	}
	if speaker.callCount() != 1 {
		t.Fatalf("speaker calls after cooldown tick = %d, want still 1", speaker.callCount())
	}
}

// TestLiveSpeakComposeFailureBacksOffAndStopsCalling drives repeated ticks
// of a composer that always errors and checks that calls follow the
// backoff schedule (5m, 10m, 20m, 40m, 60m cap) rather than firing every
// tick — the gap ticks must make zero calls.
func TestLiveSpeakComposeFailureBacksOffAndStopsCalling(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Composed: true}}, errs: []error{errors.New("boom")}}
	r := liveRuntime(t, &now, speaker, nil)
	r.deps.Observer = &fakeObserver{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute)}}
	// Force a greet decision every tick by keeping JustArrived true: easier
	// is to directly use check_in via long_absence with Reachable through
	// the console. We use arrival once, then rely on long_absence staying
	// true across backoff ticks since nothing is ever delivered.
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser:         []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryError || !e.Composed {
		t.Fatalf("first tick = %+v, want a composed error", e)
	}
	if speaker.callCount() != 1 {
		t.Fatalf("calls after tick 1 = %d, want 1", speaker.callCount())
	}
	// Minute-by-minute ticks for the next 4 minutes (well inside the first
	// 5m backoff window) must make zero additional calls.
	for i := 0; i < 4; i++ {
		now = now.Add(time.Minute)
		e := r.RunOnce(context.Background())
		if e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonBackoff {
			t.Fatalf("tick %d = %+v, want skipped/backoff", i, e)
		}
	}
	if speaker.callCount() != 1 {
		t.Fatalf("calls during the first backoff window = %d, want still 1", speaker.callCount())
	}
	// Past the 5-minute mark, the next attempt is eligible again.
	now = now.Add(2 * time.Minute)
	e2 := r.RunOnce(context.Background())
	if e2.Delivery != DeliveryError {
		t.Fatalf("tick after backoff window = %+v, want another composed attempt", e2)
	}
	if speaker.callCount() != 2 {
		t.Fatalf("calls after the backoff window = %d, want 2", speaker.callCount())
	}
}

// TestLiveSpeakDailyCapStopsComposing checks the daily speak-call cap: once
// reached, no more compose calls happen even though the decision keeps
// wanting to speak.
func TestLiveSpeakDailyCapStopsComposing(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Composed: true}}, errs: []error{errors.New("boom")}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: &fakeObserver{obs: Observation{
			ConsoleConnectedAt: now.Add(-time.Minute),
			RecentUser:         []UserMessage{{At: now.Add(-25 * time.Hour)}},
		}},
		Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now }, Speaker: speaker,
	}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 2}, deps)
	// Tick 1: compose fails (count=1). Advance past backoff (5m) each time
	// so the cap -- not backoff -- is what stops tick 3.
	for i := 0; i < 2; i++ {
		e := r.RunOnce(context.Background())
		if e.Delivery != DeliveryError {
			t.Fatalf("tick %d = %+v, want composed error", i, e)
		}
		now = now.Add(10 * time.Minute)
	}
	if speaker.callCount() != 2 {
		t.Fatalf("calls = %d, want 2 (cap)", speaker.callCount())
	}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonDailySpeakCap {
		t.Fatalf("tick 3 = %+v, want skipped/daily_speak_cap", e)
	}
	if speaker.callCount() != 2 {
		t.Fatalf("calls after cap = %d, want still 2", speaker.callCount())
	}
}

// TestLiveSpeakUnavailableWhenNoSpeakerConfigured checks that a nil Speaker
// skips without ever looking like a transient failure (no backoff, no
// compose attempt).
func TestLiveSpeakUnavailableWhenNoSpeakerConfigured(t *testing.T) {
	now := at(9, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: &fakeObserver{obs: Observation{
			ConsoleConnectedAt: now.Add(-time.Minute),
			RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
		}},
		Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now },
	}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul}, deps)
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonSpeakUnavailable || e.Composed {
		t.Fatalf("entry = %+v, want skipped/speak_unavailable with no compose attempt", e)
	}
}

// TestLiveSpeakBusyClaimSkipsWithoutCountingAsComposed checks that a
// Speak() outcome reporting "busy" (claim lost to a real chat turn, no LLM
// call made) is treated as skipped, not an error, and does not feed the
// backoff/daily-cap counters meant for actual compose attempts.
func TestLiveSpeakBusyClaimSkipsWithoutCountingAsComposed(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Reason: deliveryReasonBusy}}}
	r := liveRuntime(t, &now, speaker, nil)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonBusy || e.Composed {
		t.Fatalf("entry = %+v, want skipped/busy with Composed=false", e)
	}
	if r.hist.SpeakCallsToday != 0 || r.hist.SpeakFailCount != 0 {
		t.Fatalf("hist = %+v, a busy claim must not touch the compose counters", r.hist)
	}
}

// TestLiveSpeakRestartRestoresBackoffFromLedger checks that a fresh Runtime
// over a ledger holding consecutive compose errors resumes the backoff
// schedule instead of calling immediately.
func TestLiveSpeakRestartRestoresBackoffFromLedger(t *testing.T) {
	now := at(9, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	// Two consecutive composed errors, 1 minute apart, the second at
	// now-1m: backoff after 2 failures is 10m, so "now" is still inside
	// the window.
	_ = ledger.Append(Entry{At: now.Add(-2 * time.Minute), Mode: ModeLive,
		Decision: Decision{Intent: IntentGreet, Speak: true}, Composed: true, Delivery: DeliveryError})
	_ = ledger.Append(Entry{At: now.Add(-time.Minute), Mode: ModeLive,
		Decision: Decision{Intent: IntentGreet, Speak: true}, Composed: true, Delivery: DeliveryError})
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Composed: true}}, errs: []error{errors.New("boom")}}
	r := liveRuntime(t, &now, speaker, ledger)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonBackoff {
		t.Fatalf("entry = %+v, want skipped/backoff restored from ledger", e)
	}
	if speaker.callCount() != 0 {
		t.Fatalf("calls = %d, want 0 (still backing off)", speaker.callCount())
	}
}

// TestLiveSpeakConcurrentTicksDeliverOnlyOnce drives two concurrent
// RunOnce calls at the same instant (the mu lock must serialize them) and
// checks only one results in a delivered speak — the second must see
// cooldown, never a second compose call racing the first.
func TestLiveSpeakConcurrentTicksDeliverOnlyOnce(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := liveRuntime(t, &now, speaker, nil)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
	}}
	var wg sync.WaitGroup
	entries := make([]Entry, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entries[i] = r.RunOnce(context.Background())
		}(i)
	}
	wg.Wait()
	delivered := 0
	for _, e := range entries {
		if e.Delivery == DeliveryDelivered {
			delivered++
		}
	}
	if delivered != 1 {
		t.Fatalf("delivered count = %d, want exactly 1 (entries=%+v)", delivered, entries)
	}
	if speaker.callCount() != 1 {
		t.Fatalf("speaker calls = %d, want exactly 1", speaker.callCount())
	}
}

// TestTextReadBackoffStopsRepeatedFailuresFromCallingEveryTick drives 60
// one-minute ticks of an always-failing text-signal backend (changing text
// each tick so the hash cache never hides the fact that the call would
// otherwise be worth making) and checks the number of calls matches the
// backoff schedule's step count, not the tick count — 60 ticks over one
// hour with a 5m-start/60m-cap doubling schedule can make at most a
// handful of attempts, not 60.
func TestTextReadBackoffStopsRepeatedFailuresFromCallingEveryTick(t *testing.T) {
	now := at(9, 0)
	fake := &fakeSystemOne{err: errors.New("connection refused")}
	observer := &fakeObserver{obs: Observation{TelegramPaired: true}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: observer, SystemOne: fake, IncludeText: true, Ledger: ledger,
		Logger: zerolog.Nop(), Now: func() time.Time { return now },
	}
	r := NewRuntime(Config{Enabled: true, Location: seoul, DailyTextCalls: 60}, deps)
	for i := 0; i < 60; i++ {
		observer.obs.RecentUser = []UserMessage{{At: now.Add(-time.Hour), Text: strings.Repeat("x", i+1)}}
		r.RunOnce(context.Background())
		now = now.Add(time.Minute)
	}
	// Backoff schedule over 60 minutes starting at tick 0: attempts at
	// minute 0, 5, 15, 35 (and 55 would be the next, past 60m cap from
	// minute 35+60 -- not reached in 60 ticks starting at 0). That is at
	// most 5 calls, nowhere near 60.
	if fake.calls == 0 || fake.calls > 6 {
		t.Fatalf("calls over 60 failing ticks = %d, want a small backoff step count (<=6), not ~60", fake.calls)
	}
}

// TestSnapshotReportsDeliveryAndSpeakBackend checks the status fields
// GET /v1/initiative/status and `tars doctor` read: a delivered speak
// shows up as last_delivery/spoken_today, and speak_backend carries the
// caller-supplied BackendInfo plus live call-cap counters — all without
// ever holding the composed text.
func TestSnapshotReportsDeliveryAndSpeakBackend(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: &fakeObserver{obs: Observation{
			ConsoleConnectedAt: now.Add(-time.Minute),
			RecentUser:         []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}},
		}},
		Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now }, Speaker: speaker,
		SpeakBackend: BackendInfo{Backend: "llm", Kind: "anthropic", Provider: "shared", Model: "haiku", Tier: "light", SendsText: true, TextReason: "llm_same_provider"},
	}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 12}, deps)
	r.RunOnce(context.Background())

	snap := r.Snapshot()
	if snap.LastDelivery == nil || snap.LastDelivery.Delivery != DeliveryDelivered || snap.LastDelivery.Intent != string(IntentGreet) {
		t.Fatalf("last delivery = %+v", snap.LastDelivery)
	}
	if snap.SpokenToday != 1 {
		t.Fatalf("spoken today = %d, want 1", snap.SpokenToday)
	}
	if snap.SpeakBackend.Model != "haiku" || snap.SpeakBackend.DailyCallCap != 12 || snap.SpeakBackend.CallsToday != 1 {
		t.Fatalf("speak backend = %+v", snap.SpeakBackend)
	}
}
