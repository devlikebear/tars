package initiative

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	r, ledger := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute),
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
	r, _ := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute),
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
	r, _ := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute), Profile: "likes tea",
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
	r, _ := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-3 * time.Hour), BodyAvailable: true,
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
	r, _ := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute),
		RecentUser: []UserMessage{{At: now.Add(-10 * time.Hour)}}}, ledger: ledger}.build(t, &now)
	if e := r.RunOnce(context.Background()); e.Reason != "daily_cap" {
		t.Fatalf("restart must restore the daily cap, got %+v", e)
	}
}

func TestDailyCapResetsAtLocalMidnight(t *testing.T) {
	now := at(9, 0).Add(24 * time.Hour)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	for i := range 6 {
		_ = ledger.Append(Entry{At: at(8+i, 0), Decision: Decision{Intent: IntentGreet, Speak: true}})
	}
	r, _ := runtimeFixture{obs: Observation{ConsoleConnectedAt: now.Add(-time.Minute),
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
