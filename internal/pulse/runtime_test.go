package pulse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse/autofix"
)

// buildRuntime is a helper that wires up a runtime with a stub LLM and
// canned signal sources for orchestration tests.
func buildRuntime(t *testing.T, resp llm.ChatResponse, llmErr error) (*Runtime, *captureNotifier, *autofix.Registry) {
	t.Helper()
	scanner := buildScanner(
		ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 90}}},
		Thresholds{DiskUsedPercentWarn: 85},
		time.Now(),
	)
	decider := NewDecider(routerForClient(&fakeLLMClient{resp: resp, err: llmErr}), DeciderPolicy{
		AllowedAutofixes: []string{"compress_old_logs"},
	})
	notif := &captureNotifier{}
	router := NewNotifyRouter(notif, NotifyConfig{})
	reg := autofix.NewRegistry()
	reg.Register(&fakeAutofix{name: "compress_old_logs"})

	rt := NewRuntime(Config{Enabled: true, Interval: time.Millisecond, Timeout: time.Second},
		Dependencies{
			Scanner:   scanner,
			Decider:   decider,
			Router:    router,
			Autofixes: reg,
			State:     NewState(10),
		})
	return rt, notif, reg
}

type fakeAutofix struct {
	name    string
	changed bool
	err     error
}

func (f *fakeAutofix) Name() string { return f.name }
func (f *fakeAutofix) Run(ctx context.Context) (autofix.Result, error) {
	if f.err != nil {
		return autofix.Result{}, f.err
	}
	return autofix.Result{Name: f.name, Changed: f.changed}, nil
}

func TestRuntime_RunOnceSkipsWithoutScanner(t *testing.T) {
	rt := NewRuntime(Config{Enabled: true}, Dependencies{State: NewState(10)})
	out := rt.RunOnce(context.Background())
	if !out.Skipped || out.SkipReason != "no_scanner" {
		t.Errorf("unexpected outcome: %+v", out)
	}
}

func TestRuntime_RunOnceNoSignalsReturnsEmpty(t *testing.T) {
	// Scanner with no threshold match produces no signals.
	scanner := buildScanner(
		ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 10}}},
		Thresholds{DiskUsedPercentWarn: 85},
		time.Now(),
	)
	rt := NewRuntime(Config{Enabled: true}, Dependencies{
		Scanner: scanner,
		State:   NewState(10),
	})
	out := rt.RunOnce(context.Background())
	if len(out.Signals) != 0 {
		t.Errorf("signals = %d, want 0", len(out.Signals))
	}
	if out.DeciderInvoked {
		t.Error("decider should not run with no signals")
	}
}

func TestRuntime_RunOnceNotify(t *testing.T) {
	resp := makeToolCall(`{"action":"notify","severity":"warn","title":"Disk","summary":"high"}`)
	rt, notif, _ := buildRuntime(t, resp, nil)
	out := rt.RunOnce(context.Background())
	if !out.DeciderInvoked {
		t.Error("decider should have been invoked")
	}
	if out.Decision == nil || out.Decision.Action != ActionNotify {
		t.Errorf("decision = %+v", out.Decision)
	}
	if !out.NotifyDelivered {
		t.Error("notify should be delivered")
	}
	if notif.len() != 1 {
		t.Errorf("notifier events = %d, want 1", notif.len())
	}
}

func TestRuntime_RunOnceAutofix(t *testing.T) {
	resp := makeToolCall(`{"action":"autofix","severity":"warn","autofix_name":"compress_old_logs"}`)
	rt, _, reg := buildRuntime(t, resp, nil)
	// Replace the fake with one that marks Changed=true to verify result carry-over.
	reg.Register(&fakeAutofix{name: "compress_old_logs", changed: true})

	out := rt.RunOnce(context.Background())
	if !out.AutofixOK {
		t.Errorf("AutofixOK should be true: %+v", out)
	}
	if out.AutofixAttempt != "compress_old_logs" {
		t.Errorf("AutofixAttempt = %q", out.AutofixAttempt)
	}
	if _, ok := out.Decision.Details["autofix_result"]; !ok {
		t.Errorf("autofix_result not recorded in decision details")
	}
}

func TestRuntime_RunOnceAutofixError(t *testing.T) {
	resp := makeToolCall(`{"action":"autofix","severity":"warn","autofix_name":"compress_old_logs"}`)
	rt, _, reg := buildRuntime(t, resp, nil)
	reg.Register(&fakeAutofix{name: "compress_old_logs", err: errors.New("disk full")})

	out := rt.RunOnce(context.Background())
	if out.AutofixOK {
		t.Error("AutofixOK should be false on error")
	}
	if out.AutofixErr != "disk full" {
		t.Errorf("AutofixErr = %q", out.AutofixErr)
	}
}

func TestRuntime_RunOnceIgnore(t *testing.T) {
	resp := makeToolCall(`{"action":"ignore","severity":"info"}`)
	rt, notif, _ := buildRuntime(t, resp, nil)
	out := rt.RunOnce(context.Background())
	if out.Decision == nil || out.Decision.Action != ActionIgnore {
		t.Errorf("decision = %+v", out.Decision)
	}
	if notif.len() != 0 {
		t.Errorf("ignore should not notify")
	}
	if out.AutofixAttempt != "" {
		t.Errorf("ignore should not autofix")
	}
}

func TestRuntime_RunOnceDeciderError(t *testing.T) {
	rt, _, _ := buildRuntime(t, llm.ChatResponse{}, errors.New("timeout"))
	out := rt.RunOnce(context.Background())
	if out.Err == "" {
		t.Error("expected decider error recorded")
	}
}

func TestRuntime_StateSnapshotReflectsTicks(t *testing.T) {
	resp := makeToolCall(`{"action":"ignore","severity":"info"}`)
	rt, _, _ := buildRuntime(t, resp, nil)
	rt.RunOnce(context.Background())
	rt.RunOnce(context.Background())
	snap := rt.Snapshot()
	if snap.TotalTicks != 2 {
		t.Errorf("TotalTicks = %d, want 2", snap.TotalTicks)
	}
	if snap.TotalDecisions != 2 {
		t.Errorf("TotalDecisions = %d, want 2", snap.TotalDecisions)
	}
}

func TestRuntime_StartStop(t *testing.T) {
	resp := makeToolCall(`{"action":"ignore","severity":"info"}`)
	rt, _, _ := buildRuntime(t, resp, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	// Wait up to 500ms for at least one tick.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if rt.Snapshot().TotalTicks > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rt.Snapshot().TotalTicks == 0 {
		t.Fatal("expected at least one tick")
	}
	rt.Stop()
	// After Stop, a second Stop should be safe.
	rt.Stop()
}

func TestRuntime_StartDisabledIsNoop(t *testing.T) {
	rt := NewRuntime(Config{Enabled: false}, Dependencies{State: NewState(10)})
	rt.Start(context.Background())
	if rt.Snapshot().TotalTicks != 0 {
		t.Error("disabled runtime should not tick")
	}
	rt.Stop() // must not panic
}

func TestRuntime_StartIdempotent(t *testing.T) {
	resp := makeToolCall(`{"action":"ignore","severity":"info"}`)
	rt, _, _ := buildRuntime(t, resp, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)
	rt.Start(ctx) // should be a no-op, not a panic
	rt.Stop()
}

func TestRuntime_SetTickHookReceivesOutcomes(t *testing.T) {
	resp := makeToolCall(`{"action":"ignore","severity":"info"}`)
	rt, _, _ := buildRuntime(t, resp, nil)
	var received int
	rt.SetTickHook(func(outcome TickOutcome) { received++ })
	rt.RunOnce(context.Background())
	rt.RunOnce(context.Background())
	if received != 2 {
		t.Errorf("hook received %d, want 2", received)
	}
}

func TestRuntime_NilReceiverSafe(t *testing.T) {
	var rt *Runtime
	rt.Start(context.Background())
	rt.Stop()
	if snap := rt.Snapshot(); snap.TotalTicks != 0 {
		t.Errorf("nil Snapshot = %+v", snap)
	}
	if out := rt.RunOnce(context.Background()); out.DeciderInvoked {
		t.Error("nil RunOnce should not invoke decider")
	}
}

// --- active hours ---

func TestRuntime_ActiveHoursAlwaysOn(t *testing.T) {
	rt := &Runtime{cfg: Config{ActiveHours: "00:00-24:00"}}
	ok, _ := rt.withinActiveHours(time.Now())
	if !ok {
		t.Error("00:00-24:00 should always be active")
	}
}

func TestRuntime_ActiveHoursWithinWindow(t *testing.T) {
	rt := &Runtime{cfg: Config{ActiveHours: "09:00-17:00", Timezone: "UTC"}}
	ok, _ := rt.withinActiveHours(time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC))
	if !ok {
		t.Error("10:30 should be in 09:00-17:00")
	}
}

func TestRuntime_ActiveHoursOutsideWindow(t *testing.T) {
	rt := &Runtime{cfg: Config{ActiveHours: "09:00-17:00", Timezone: "UTC"}}
	ok, reason := rt.withinActiveHours(time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC))
	if ok {
		t.Error("20:00 should be outside 09:00-17:00")
	}
	if reason != "outside_active_hours" {
		t.Errorf("reason = %q", reason)
	}
}

func TestRuntime_ActiveHoursWrapAroundMidnight(t *testing.T) {
	rt := &Runtime{cfg: Config{ActiveHours: "22:00-02:00", Timezone: "UTC"}}
	cases := []struct {
		hour int
		want bool
	}{
		{23, true},
		{1, true},
		{2, false},
		{12, false},
	}
	for _, c := range cases {
		ok, _ := rt.withinActiveHours(time.Date(2026, 4, 5, c.hour, 0, 0, 0, time.UTC))
		if ok != c.want {
			t.Errorf("hour %d: got %v, want %v", c.hour, ok, c.want)
		}
	}
}

func TestRuntime_ActiveHoursMalformedIsAlwaysOn(t *testing.T) {
	rt := &Runtime{cfg: Config{ActiveHours: "not a window"}}
	ok, _ := rt.withinActiveHours(time.Now())
	if !ok {
		t.Error("malformed window should default to active")
	}
}

func TestParseActiveHoursValid(t *testing.T) {
	start, end, err := parseActiveHours("02:00-05:00")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if start != 120 || end != 300 {
		t.Errorf("start=%d end=%d, want 120/300", start, end)
	}
}

func TestParseActiveHoursInvalid(t *testing.T) {
	cases := []string{"", "09:00", "09-17", "99:00-10:00", "09:60-10:00", "09:00_10:00"}
	for _, c := range cases {
		if _, _, err := parseActiveHours(c); err == nil {
			t.Errorf("parseActiveHours(%q) should error", c)
		}
	}
}
