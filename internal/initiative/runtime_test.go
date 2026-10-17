package initiative

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

type fakeObserver struct {
	obs Observation
	err error
}

func (f *fakeObserver) Observe(_ context.Context, now time.Time) (Observation, error) {
	o := f.obs
	o.Now = now
	return o, f.err
}

type fakeBody struct{ calls []string }

func (f *fakeBody) Express(_ context.Context, provider, emotion string) (string, error) {
	f.calls = append(f.calls, provider+":"+emotion)
	return "delivered", nil
}

type runtimeFixture struct {
	obs     Observation
	one     SystemOne
	include bool
	body    BodyActor
	ledger  *Ledger
}

func (fx runtimeFixture) build(t *testing.T, now *time.Time) (*Runtime, *Ledger) {
	t.Helper()
	ledger := fx.ledger
	if ledger == nil {
		ledger = OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	}
	deps := Dependencies{
		Observer: &fakeObserver{obs: fx.obs}, IncludeText: fx.include,
		Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return *now },
	}
	if fx.one != nil {
		deps.SystemOne = fx.one
	}
	if fx.body != nil {
		deps.Body = fx.body
	}
	return NewRuntime(Config{Enabled: true, Location: seoul, BodyProvider: "stackchan"}, deps), ledger
}

func TestRunOnceGreetsOnArrivalInShadow(t *testing.T) {
	now := at(9, 0)
	r, ledger := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}}}, include: true}.build(t, &now)
	e := r.RunOnce(context.Background())
	if e.Intent != IntentGreet || !e.Speak || e.Mode != ModeShadow || e.Reason != "arrived" {
		t.Fatalf("entry = %+v", e)
	}
	now = now.Add(time.Minute)
	if e2 := r.RunOnce(context.Background()); e2.Reason != "cooldown" {
		t.Fatalf("second tick should be in cooldown, got %+v", e2)
	}
	now = now.Add(time.Minute)
	r.RunOnce(context.Background()) // same reason again: not recorded
	recent, _ := ledger.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("ledger entries = %d, want 2", len(recent))
	}
}

func TestRunOnceRemoteBackendSendsNoUserText(t *testing.T) {
	now := at(14, 0)
	fake := &fakeSystemOne{probs: map[string]float64{}}
	r, _ := runtimeFixture{obs: Observation{RecentUser: []UserMessage{{At: now.Add(-20 * time.Minute), Text: "private words"}},
		Profile: "birthday"}, one: fake, include: false}.build(t, &now)
	e := r.RunOnce(context.Background())
	if fake.calls != 0 || e.Text.Source != "skipped" || strings.Contains(fake.state, "private") {
		t.Fatalf("remote backend must not be asked about text: calls=%d entry=%+v", fake.calls, e)
	}
}

func TestRunOnceQuietRequestFromSystemOne(t *testing.T) {
	now := at(14, 0)
	fake := &fakeSystemOne{probs: map[string]float64{"quiet_requested": 0.9}}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-90 * time.Minute), Text: "오늘은 메시지 그만"}}}, one: fake, include: true}.build(t, &now)
	e := r.RunOnce(context.Background())
	if e.Reason != "quiet_requested" || e.Speak || !strings.Contains(fake.state, "오늘은 메시지 그만") {
		t.Fatalf("entry = %+v state=%q", e, fake.state)
	}
}

func TestRunOnceSkipsSystemOneUnderHardLimits(t *testing.T) {
	now := at(23, 30)
	fake := &fakeSystemOne{probs: map[string]float64{}}
	r, _ := runtimeFixture{obs: Observation{RecentUser: []UserMessage{{At: now.Add(-time.Hour), Text: "hi"}}}, one: fake, include: true}.build(t, &now)
	if e := r.RunOnce(context.Background()); e.Reason != "quiet_hours" || fake.calls != 0 {
		t.Fatalf("entry = %+v calls=%d", e, fake.calls)
	}
}

func TestRunOnceSystemOneErrorKeepsGoing(t *testing.T) {
	now := at(14, 0)
	fake := &fakeSystemOne{err: errors.New("connection refused")}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute), Profile: "likes tea",
		RecentUser: []UserMessage{{At: now.Add(-5 * time.Hour), Text: "hi"}}}, one: fake, include: true}.build(t, &now)
	e := r.RunOnce(context.Background())
	if e.Intent != IntentGreet || e.Text.Source != "error" || !strings.Contains(e.Error, "connection refused") {
		t.Fatalf("entry = %+v", e)
	}
	if snap := r.Snapshot(); !strings.Contains(snap.LastError, "connection refused") || len(snap.Recent) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestRunOnceObserveErrorIsReported(t *testing.T) {
	now := at(14, 0)
	r := NewRuntime(Config{Enabled: true, Location: seoul}, Dependencies{
		Observer: &fakeObserver{err: errors.New("disk gone")}, Logger: zerolog.Nop(), Now: func() time.Time { return now },
	})
	if e := r.RunOnce(context.Background()); e.Reason != "observe_error" || r.Snapshot().LastError != "disk gone" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestRunOnceBodyOnlyExpressesAndIsSpaced(t *testing.T) {
	now := at(14, 0)
	body := &fakeBody{}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-3 * time.Hour), BodyAvailable: true,
		RecentUser: []UserMessage{{At: now.Add(-20 * time.Minute)}}}, body: body, include: true}.build(t, &now)
	if e := r.RunOnce(context.Background()); e.Intent != IntentBodyOnly || e.Body != "delivered" {
		t.Fatalf("entry = %+v", e)
	}
	now = now.Add(5 * time.Minute)
	if e := r.RunOnce(context.Background()); e.Intent != IntentNone {
		t.Fatalf("body expression must be spaced, got %+v", e)
	}
	if len(body.calls) != 1 || body.calls[0] != "stackchan:happy" {
		t.Fatalf("body calls = %v", body.calls)
	}
}

func TestRuntimeRestoresHistoryFromLedger(t *testing.T) {
	now := at(15, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	for i := range 6 {
		_ = ledger.Append(Entry{At: at(8+i, 0), Decision: Decision{Intent: IntentCheckIn, Speak: true}})
	}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-10 * time.Hour)}}}, ledger: ledger}.build(t, &now)
	if e := r.RunOnce(context.Background()); e.Reason != "daily_cap" {
		t.Fatalf("restart must restore the daily cap, got %+v", e)
	}
}

// TestRepeatedTicksOnUnchangedObservationCallBackendOnce drives 100 ticks of
// an unchanged observation and expects exactly one backend call: the hash
// cache stops the second tick onward from asking again (same key), and once
// the first tick speaks, the cooldown gate also makes the decision stop
// depending on text at all. Either mechanism alone would already hold calls
// at 1; this asserts the combined behavior guards against the "870 calls in
// 16 hours" class of regression (CLAUDE.md pulse incident).
func TestRepeatedTicksOnUnchangedObservationCallBackendOnce(t *testing.T) {
	now := at(9, 0)
	fake := &fakeSystemOne{probs: map[string]float64{}}
	// The user message is outside RenderState's 2h text window on
	// purpose, so only the profile line keeps the state's hash key
	// non-empty — this fixture is otherwise identical to
	// TestRunOnceGreetsOnArrivalInShadow.
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute), Profile: "likes tea",
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour), Text: "잘 자"}}}, one: fake, include: true}.build(t, &now)
	for range 100 {
		r.RunOnce(context.Background())
	}
	if fake.calls != 1 {
		t.Fatalf("expected exactly one backend call across 100 unchanged ticks, got %d", fake.calls)
	}
}

// TestTextSignalsMatterGatesPreventAnyCall exercises each hard-rule gate
// through RunOnce (not just the pure textSignalsMatter unit test), proving
// zero backend calls happen in every case the issue calls out: quiet
// hours, cooldown, the daily initiative cap, and plain busy typing with no
// arrival.
func TestTextSignalsMatterGatesPreventAnyCall(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		obs  Observation
	}{
		{"quiet hours", at(23, 30), Observation{TelegramPaired: true}},
		// 14:00 is outside the default quiet_hours window, so this case
		// isolates the Busy gate itself rather than riding along on
		// quiet_hours already forcing 0 calls.
		{"busy typing, not arriving", at(14, 0), Observation{ChatBusy: true, TelegramPaired: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			now := c.now
			fake := &fakeSystemOne{probs: map[string]float64{}}
			r, _ := runtimeFixture{obs: c.obs, one: fake, include: true}.build(t, &now)
			r.RunOnce(context.Background())
			if fake.calls != 0 {
				t.Fatalf("%s: expected 0 backend calls, got %d", c.name, fake.calls)
			}
		})
	}

	t.Run("cooldown", func(t *testing.T) {
		now := at(9, 0)
		ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
		_ = ledger.Append(Entry{At: now.Add(-time.Minute), Decision: Decision{Intent: IntentGreet, Speak: true}})
		fake := &fakeSystemOne{probs: map[string]float64{}}
		r, _ := runtimeFixture{obs: Observation{TelegramPaired: true}, one: fake, include: true, ledger: ledger}.build(t, &now)
		r.RunOnce(context.Background())
		if fake.calls != 0 {
			t.Fatalf("cooldown: expected 0 backend calls, got %d", fake.calls)
		}
	})

	t.Run("daily cap", func(t *testing.T) {
		now := at(9, 0)
		ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
		for i := range 6 {
			_ = ledger.Append(Entry{At: at(8+i, 0), Decision: Decision{Intent: IntentGreet, Speak: true}})
		}
		fake := &fakeSystemOne{probs: map[string]float64{}}
		r, _ := runtimeFixture{obs: Observation{TelegramPaired: true}, one: fake, include: true, ledger: ledger}.build(t, &now)
		r.RunOnce(context.Background())
		if fake.calls != 0 {
			t.Fatalf("daily cap: expected 0 backend calls, got %d", fake.calls)
		}
	})
}

// TestDailyTextCallCapStopsCallingButKeepsDeciding drives ticks with
// different text each time (defeating the hash cache) past a small daily
// text-call cap and checks the backend stops being called, the cap shows
// up on Snapshot, and a fresh runtime over the same ledger (a restart)
// restores today's count instead of calling again.
func TestDailyTextCallCapStopsCallingButKeepsDeciding(t *testing.T) {
	now := at(9, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	fake := &fakeSystemOne{probs: map[string]float64{}}
	observer := &fakeObserver{obs: Observation{TelegramPaired: true}}
	deps := Dependencies{Observer: observer, SystemOne: fake, IncludeText: true, Ledger: ledger,
		Logger: zerolog.Nop(), Now: func() time.Time { return now }}
	r := NewRuntime(Config{Enabled: true, Location: seoul, DailyTextCalls: 2}, deps)
	for i := range 5 {
		observer.obs.RecentUser = []UserMessage{{At: now.Add(-time.Hour), Text: strings.Repeat("x", i+1)}}
		r.RunOnce(context.Background())
	}
	if fake.calls != 2 {
		t.Fatalf("expected calls capped at 2, got %d", fake.calls)
	}
	if snap := r.Snapshot(); snap.Backend.CallsToday != 2 || snap.Backend.DailyCallCap != 2 {
		t.Fatalf("snapshot backend = %+v, want CallsToday=2 DailyCallCap=2", snap.Backend)
	}

	fake2 := &fakeSystemOne{probs: map[string]float64{}}
	observer2 := &fakeObserver{obs: Observation{TelegramPaired: true,
		RecentUser: []UserMessage{{At: now.Add(-time.Hour), Text: "after restart"}}}}
	deps2 := Dependencies{Observer: observer2, SystemOne: fake2, IncludeText: true, Ledger: ledger,
		Logger: zerolog.Nop(), Now: func() time.Time { return now }}
	r2 := NewRuntime(Config{Enabled: true, Location: seoul, DailyTextCalls: 2}, deps2)
	r2.RunOnce(context.Background())
	if fake2.calls != 0 {
		t.Fatalf("a restart must restore today's call count from the ledger, got %d calls", fake2.calls)
	}
}

// TestDailyTextCallCapResetsAtLocalMidnight mirrors
// TestDailyCapResetsAtLocalMidnight for the text-call cap: ledger entries
// from "yesterday" must not count against today.
func TestDailyTextCallCapResetsAtLocalMidnight(t *testing.T) {
	now := at(9, 0).Add(24 * time.Hour)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	for i := range 2 {
		_ = ledger.Append(Entry{At: at(8+i, 0), Called: true})
	}
	fake := &fakeSystemOne{probs: map[string]float64{}}
	r, _ := runtimeFixture{obs: Observation{TelegramPaired: true,
		RecentUser: []UserMessage{{At: now.Add(-time.Hour), Text: "hi"}}}, one: fake, include: true, ledger: ledger}.build(t, &now)
	r.cfg.DailyTextCalls = 2
	r.RunOnce(context.Background())
	if fake.calls != 1 {
		t.Fatalf("a new day must reset the text-call cap, got %d calls", fake.calls)
	}
}

func TestDailyCapResetsAtLocalMidnight(t *testing.T) {
	now := at(9, 0).Add(24 * time.Hour)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	for i := range 6 {
		_ = ledger.Append(Entry{At: at(8+i, 0), Decision: Decision{Intent: IntentGreet, Speak: true}})
	}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-10 * time.Hour)}}}, ledger: ledger}.build(t, &now)
	if e := r.RunOnce(context.Background()); e.Intent != IntentGreet {
		t.Fatalf("a new day must reset the cap, got %+v", e)
	}
}

func TestStartStop(t *testing.T) {
	now := at(14, 0)
	r, _ := runtimeFixture{}.build(t, &now)
	r.cfg.Tick = time.Millisecond
	ctx := t.Context()
	r.Start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for len(r.Snapshot().Recent) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	r.Stop()
	if len(r.Snapshot().Recent) == 0 {
		t.Fatal("loop never ticked")
	}
	r.Stop() // idempotent

	disabled := NewRuntime(Config{}, Dependencies{Observer: &fakeObserver{}, Logger: zerolog.Nop()})
	disabled.Start(ctx)
	disabled.Stop() // must not block when never started
}

type blockingSystemOne struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingSystemOne) Ask(ctx context.Context, _ string, _ map[string]jev.Question) (jev.Response, error) {
	close(b.entered)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return jev.Response{}, nil
}

func TestSnapshotIsNotBlockedByASlowSystemOne(t *testing.T) {
	now := at(14, 0)
	slow := &blockingSystemOne{entered: make(chan struct{}), release: make(chan struct{})}
	// TelegramPaired makes the user reachable, so the strained-check-in
	// branch depends on the text signal and the backend is worth calling
	// (textSignalsMatter) — the point of this test is the concurrent
	// Snapshot call while that happens, not this particular go-signal mix.
	r, _ := runtimeFixture{obs: Observation{Profile: "likes tea", TelegramPaired: true,
		RecentUser: []UserMessage{{At: now.Add(-time.Hour), Text: "hi"}}},
		one: slow, include: true}.build(t, &now)
	done := make(chan struct{})
	go func() { r.RunOnce(context.Background()); close(done) }()
	<-slow.entered
	got := make(chan Snapshot, 1)
	go func() { got <- r.Snapshot() }()
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("Snapshot blocked while the System One was answering")
	}
	close(slow.release)
	<-done
}

type failingBody struct{ calls int }

func (f *failingBody) Express(context.Context, string, string) (string, error) {
	f.calls++
	return "dropped:provider_disabled", nil
}

func TestFailedBodyExpressionIsSpacedToo(t *testing.T) {
	now := at(14, 0)
	body := &failingBody{}
	r, _ := runtimeFixture{obs: Observation{ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-3 * time.Hour), BodyAvailable: true,
		RecentUser: []UserMessage{{At: now.Add(-20 * time.Minute)}}}, body: body, include: true}.build(t, &now)
	r.RunOnce(context.Background())
	now = now.Add(5 * time.Minute)
	r.RunOnce(context.Background())
	if body.calls != 1 {
		t.Fatalf("a failed expression must not be retried every tick, calls=%d", body.calls)
	}
}

func TestSystemOneErrorBodyIsNotRecorded(t *testing.T) {
	now := at(14, 0)
	fake := &fakeSystemOne{err: &jev.HTTPError{Status: 422, Message: `{"detail":[{"input":"recent_user_messages: 오늘 너무 피곤해"}]}`}}
	r, ledger := runtimeFixture{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute), Profile: "likes tea",
		RecentUser: []UserMessage{{At: now.Add(-5 * time.Hour), Text: "오늘 너무 피곤해"}}}, one: fake, include: true}.build(t, &now)
	e := r.RunOnce(context.Background())
	if strings.Contains(e.Error, "피곤") || !strings.Contains(e.Error, "422") {
		t.Fatalf("entry error = %q", e.Error)
	}
	recent, _ := ledger.Recent(10)
	for _, entry := range recent {
		if strings.Contains(entry.Error, "피곤") {
			t.Fatalf("ledger holds user text: %q", entry.Error)
		}
	}
	if strings.Contains(r.Snapshot().LastError, "피곤") {
		t.Fatal("status holds user text")
	}
}

// TestLLMProviderErrorBodyIsNotRecorded mirrors
// TestSystemOneErrorBodyIsNotRecorded for the llm backend: a provider's
// non-2xx response body can echo the request (which, on a same-provider
// backend, holds the user's own words), and that body must not reach the
// ledger, the logs, or GET /v1/initiative/status (tars#1219).
func TestLLMProviderErrorBodyIsNotRecorded(t *testing.T) {
	now := at(14, 0)
	client := &stubChatClient{err: &llm.ProviderError{
		Provider:   "openai",
		StatusCode: 400,
		Message:    `{"error":{"message":"content flagged: 오늘 너무 피곤해"}}`,
	}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: &fakeObserver{obs: Observation{ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute), Profile: "likes tea",
			RecentUser: []UserMessage{{At: now.Add(-5 * time.Hour), Text: "오늘 너무 피곤해"}}}},
		TextLLM: client, IncludeText: true, Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now },
	}
	r := NewRuntime(Config{Enabled: true, Location: seoul}, deps)
	e := r.RunOnce(context.Background())
	if strings.Contains(e.Error, "피곤") || !strings.Contains(e.Error, "400") {
		t.Fatalf("entry error = %q", e.Error)
	}
	recent, _ := ledger.Recent(10)
	for _, entry := range recent {
		if strings.Contains(entry.Error, "피곤") {
			t.Fatalf("ledger holds user text: %q", entry.Error)
		}
	}
	if strings.Contains(r.Snapshot().LastError, "피곤") {
		t.Fatal("status holds user text")
	}
}

// TestRunOnceGreetedTodayBlocksSecondArrivalSameDay drives a shadow-mode
// runtime through a greeted arrival, a second arrival later the same local
// day, and the next local day's first arrival (tars#1269). The second
// arrival must not greet, but — unlike the other hard gates — it must not
// record its own "none(greeted_today)" either: with nothing else pending,
// Decide falls all the way through to "nothing", and GoSignals.GreetedToday
// on the recorded entry is what actually shows the greet was suppressed.
func TestRunOnceGreetedTodayBlocksSecondArrivalSameDay(t *testing.T) {
	now := at(9, 0)
	observer := &fakeObserver{obs: Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour)}},
	}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{Observer: observer, Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now }}
	r := NewRuntime(Config{Enabled: true, Location: seoul}, deps)

	e1 := r.RunOnce(context.Background())
	if e1.Intent != IntentGreet || e1.Reason != "arrived" || e1.Signals.GreetedToday {
		t.Fatalf("first arrival = %+v", e1)
	}

	// A second arrival 7h later, well past the 45m default cooldown and
	// the 4h arrival gap, with nothing else (not busy, no long session or
	// absence, not idle at the desk) pending.
	now = at(16, 0)
	observer.obs.ConsolePresentAt = now.Add(-time.Minute)
	observer.obs.ConsoleArrivedAt = now.Add(-time.Minute)
	observer.obs.RecentUser = []UserMessage{{At: now.Add(-14 * time.Hour)}}
	e2 := r.RunOnce(context.Background())
	if e2.Intent != IntentNone || e2.Reason != "nothing" || !e2.Signals.GreetedToday {
		t.Fatalf("second arrival same day = %+v, want none/nothing with GreetedToday recorded", e2)
	}

	// The next local day's first arrival greets again.
	now = at(9, 0).Add(24 * time.Hour)
	observer.obs.ConsolePresentAt = now.Add(-time.Minute)
	observer.obs.ConsoleArrivedAt = now.Add(-time.Minute)
	observer.obs.RecentUser = []UserMessage{{At: now.Add(-14 * time.Hour)}}
	e3 := r.RunOnce(context.Background())
	if e3.Intent != IntentGreet || e3.Reason != "arrived" || e3.Signals.GreetedToday {
		t.Fatalf("next day's first arrival = %+v", e3)
	}
}

// TestRunOnceGreetedTodayStillAllowsOtherDecisions checks that a second
// same-day arrival still reaches check_in/body_only when the rest of the
// tick's signals call for it (tars#1269 requirement: GreetedToday is not a
// hard gate like StoppedToday/QuietHours — it only narrows Decide's arrival
// case).
func TestRunOnceGreetedTodayStillAllowsOtherDecisions(t *testing.T) {
	now := at(9, 0)
	observer := &fakeObserver{obs: Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour)}},
	}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	fake := &fakeSystemOne{probs: map[string]float64{"user_strained": 0.9}}
	deps := Dependencies{Observer: observer, SystemOne: fake, IncludeText: true, Ledger: ledger,
		Logger: zerolog.Nop(), Now: func() time.Time { return now }}
	r := NewRuntime(Config{Enabled: true, Location: seoul}, deps)

	if e := r.RunOnce(context.Background()); e.Intent != IntentGreet {
		t.Fatalf("first arrival = %+v", e)
	}

	// A second arrival, still the same local day, but the user now reads
	// as strained (and has not been checked in on recently): the arrival
	// case is unmatched (GreetedToday), so the switch falls through to
	// the strain check-in case instead of stopping at "nothing".
	now = at(16, 0)
	observer.obs.ConsolePresentAt = now.Add(-time.Minute)
	observer.obs.ConsoleArrivedAt = now.Add(-time.Minute)
	observer.obs.RecentUser = []UserMessage{{At: now.Add(-time.Hour), Text: "힘들다"}}
	e := r.RunOnce(context.Background())
	if e.Intent != IntentCheckIn || e.Reason != "strained" || !e.Signals.GreetedToday {
		t.Fatalf("second arrival with strain = %+v, want check_in/strained with GreetedToday recorded", e)
	}
}

// TestLiveGreetSkippedDeliveryDoesNotBlockNextArrival checks requirement
// #1/#2's live-mode rule from the ledger entry's own documentation: only an
// actually Delivered greet counts toward GreetedToday. With no Speaker
// configured, deliverLocked always records Delivery=skipped,
// reason=speak_unavailable and never actually says anything — so a second
// arrival the same day must still be free to greet (tars#1269).
func TestLiveGreetSkippedDeliveryDoesNotBlockNextArrival(t *testing.T) {
	now := at(9, 0)
	observer := &fakeObserver{obs: Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour)}},
	}}
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{Observer: observer, Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return now }}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul}, deps)

	e1 := r.RunOnce(context.Background())
	if e1.Intent != IntentGreet || e1.Delivery != DeliverySkipped || e1.DeliveryReason != deliveryReasonSpeakUnavailable {
		t.Fatalf("first tick = %+v", e1)
	}

	now = now.Add(2 * time.Hour)
	observer.obs.ConsolePresentAt = now.Add(-time.Minute)
	observer.obs.ConsoleArrivedAt = now.Add(-time.Minute)
	observer.obs.RecentUser = []UserMessage{{At: now.Add(-14 * time.Hour)}}
	e2 := r.RunOnce(context.Background())
	if e2.Intent != IntentGreet || e2.Delivery != DeliverySkipped || e2.DeliveryReason != deliveryReasonSpeakUnavailable {
		t.Fatalf("second arrival same day = %+v, want another skipped greet attempt (not blocked)", e2)
	}
}

// TestLiveGreetDeliveredBlocksSecondArrivalSameDay is
// TestLiveGreetSkippedDeliveryDoesNotBlockNextArrival's counterpart: once a
// greet is actually delivered, a second arrival the same day does not
// deliver another one.
func TestLiveGreetDeliveredBlocksSecondArrivalSameDay(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := liveRuntime(t, &now, speaker, nil)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour)}},
	}}

	e1 := r.RunOnce(context.Background())
	if e1.Intent != IntentGreet || e1.Delivery != DeliveryDelivered {
		t.Fatalf("first tick = %+v", e1)
	}

	now = now.Add(2 * time.Hour)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-14 * time.Hour)}},
	}}
	e2 := r.RunOnce(context.Background())
	if e2.Intent != IntentNone || e2.Reason != "nothing" || !e2.Signals.GreetedToday {
		t.Fatalf("second arrival same day = %+v, want none/nothing with GreetedToday recorded", e2)
	}
	if speaker.callCount() != 1 {
		t.Fatalf("speaker calls = %d, want exactly 1 (second arrival must not compose again)", speaker.callCount())
	}
}

// TestGreetedTodayRestoresAfterRestart checks requirement #4: a fresh
// Runtime over a ledger that already has today's delivered greet restores
// GreetedToday from historyFrom and blocks a same-day arrival, exactly as
// the still-running original runtime would.
func TestGreetedTodayRestoresAfterRestart(t *testing.T) {
	now := at(9, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	if err := ledger.Append(Entry{At: now, Mode: ModeLive, Decision: Decision{Intent: IntentGreet, Speak: true, Reason: "arrived"}, Delivery: DeliveryDelivered}); err != nil {
		t.Fatal(err)
	}

	restartNow := at(16, 0)
	observer := &fakeObserver{obs: Observation{
		ConsolePresentAt: restartNow.Add(-time.Minute), ConsoleArrivedAt: restartNow.Add(-time.Minute),
		RecentUser: []UserMessage{{At: restartNow.Add(-14 * time.Hour)}},
	}}
	deps := Dependencies{Observer: observer, Ledger: ledger, Logger: zerolog.Nop(), Now: func() time.Time { return restartNow }}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul}, deps)

	e := r.RunOnce(context.Background())
	if e.Intent != IntentNone || e.Reason != "nothing" || !e.Signals.GreetedToday {
		t.Fatalf("restarted runtime = %+v, want none/nothing with GreetedToday restored from the ledger", e)
	}
}
