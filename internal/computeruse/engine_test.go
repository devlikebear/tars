package computeruse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/jev"
)

// FakeDriver serves scripted snapshots in order (the last one repeats) and
// records every action.
type FakeDriver struct {
	snaps   []Snapshot
	i       int
	actions []string
	effect  Effect
	pingErr error
	opts    []SnapshotOpts
}

func (f *FakeDriver) Ping(context.Context) error { return f.pingErr }
func (f *FakeDriver) ResolveWindow(context.Context, string) (Window, error) {
	return Window{PID: 1, WindowID: 2, App: "App", Title: "Win"}, nil
}
func (f *FakeDriver) Snapshot(_ context.Context, w Window, opts SnapshotOpts) (Snapshot, error) {
	f.opts = append(f.opts, opts)
	s := f.snaps[f.i]
	if f.i < len(f.snaps)-1 {
		f.i++
	}
	s.Window = w
	return s, nil
}
func (f *FakeDriver) act(kind string) (Effect, error) {
	f.actions = append(f.actions, kind)
	if f.effect == "" {
		return EffectConfirmed, nil
	}
	return f.effect, nil
}
func (f *FakeDriver) Click(_ context.Context, _ Window, tok string) (Effect, error) {
	return f.act("click:" + tok)
}
func (f *FakeDriver) SetValue(_ context.Context, _ Window, tok, v string) (Effect, error) {
	return f.act("set:" + tok + "=" + v)
}
func (f *FakeDriver) TypeText(_ context.Context, _ Window, tok, v string) (Effect, error) {
	return f.act("type:" + tok + "=" + v)
}
func (f *FakeDriver) PressKey(_ context.Context, _ Window, k string) (Effect, error) {
	return f.act("key:" + k)
}
func (f *FakeDriver) Scroll(_ context.Context, _ Window, d string) (Effect, error) {
	return f.act("scroll:" + d)
}

// FakeAsker replays decisions in order (last repeats) and captures states.
type FakeAsker struct {
	answers []map[string]jev.Answer
	i       int
	states  []string
	err     error
}

func (f *FakeAsker) Ask(_ context.Context, state string, _ map[string]jev.Question) (jev.Response, error) {
	f.states = append(f.states, state)
	if f.err != nil {
		return jev.Response{}, f.err
	}
	a := f.answers[f.i]
	if f.i < len(f.answers)-1 {
		f.i++
	}
	return jev.Response{Model: "fake", Answers: a, Usage: jev.Usage{InputTokens: 100}}, nil
}

func decide(op, target string, conf, risky, done float64) map[string]jev.Answer {
	return map[string]jev.Answer{
		"op":     {Type: "choice", Choice: op, Confidence: conf},
		"target": {Type: "choice", Choice: target, Confidence: conf},
		"risky":  {Type: "noul", Noul: risky},
		"done":   {Type: "noul", Noul: done},
	}
}

func withInput(a map[string]jev.Answer, key string) map[string]jev.Answer {
	a["input_key"] = jev.Answer{Type: "choice", Choice: key, Confidence: 0.9}
	return a
}

func snap(labels ...string) Snapshot {
	s := Snapshot{TotalElements: len(labels)}
	for i, l := range labels {
		role := "AXButton"
		if l == "Name" {
			role = "AXTextField"
		}
		s.Elements = append(s.Elements, Element{Index: i + 1, Token: "t" + string(rune('0'+i+1)), Role: role, Label: l, Enabled: true})
	}
	return s
}

func newTestEngine(d *FakeDriver, a *FakeAsker) *Engine {
	cfg := DefaultConfig()
	cfg.MaxSteps = 6
	e := NewEngine(d, a, cfg)
	e.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	e.newToken = func() string { return "cu_test" }
	return e
}

func TestRun_ClickThenDone(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("New Folder"), snap("New Folder", "untitled folder")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.9, 0.05, 0.02), decide("done", "none", 0.9, 0.0, 0.95)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "make folder"})
	if res.Status != StatusDone || res.Steps != 2 || len(d.actions) != 1 || d.actions[0] != "click:t1" {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
	if res.Trace[0].Target != "AXButton 'New Folder'" || res.Trace[0].Effect != "confirmed" || res.Usage.JevInputTokens != 200 {
		t.Fatalf("trace=%+v usage=%+v", res.Trace, res.Usage)
	}
}

func TestRun_TypeUsesInputValueButStateOnlyShowsKey(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("Name"), snap("Name")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{withInput(decide("type", "e1", 0.9, 0.05, 0.0), "name"), decide("done", "none", 0.9, 0, 0.99)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g", Inputs: map[string]string{"name": "s3cret"}})
	if res.Status != StatusDone || d.actions[0] != "type:t1=s3cret" {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
	for _, s := range a.states {
		if contains(s, "s3cret") {
			t.Fatal("input value leaked to Jev state")
		}
	}
	if contains(res.Trace[0].Note, "s3cret") || contains(res.Trace[0].Target, "s3cret") {
		t.Fatal("input value leaked to trace")
	}
}

func TestRun_NoneThreeTimesIsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("none", "none", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || res.Steps != 3 || len(d.actions) != 0 {
		t.Fatalf("res=%+v", res)
	}
}

func TestRun_LowConfidenceLooksOnceThenStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.4, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	sawLook := false
	for _, o := range d.opts {
		if o.MaxDepth > 0 {
			sawLook = true
		}
	}
	if res.Status != StatusStuck || !sawLook || len(d.actions) != 0 {
		t.Fatalf("res=%+v opts=%+v actions=%v", res, d.opts, d.actions)
	}
	if res.Trace[0].Note != "low_confidence_look" {
		t.Fatalf("trace[0]=%+v", res.Trace[0])
	}
}

func TestRun_RiskyStopsAndResumeContinues(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("Delete"), snap("Delete")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.95, 0.8, 0), decide("done", "none", 0.9, 0, 0.99)}}
	e := newTestEngine(d, a)
	res := e.Run(context.Background(), Request{Goal: "delete it"})
	if res.Status != StatusNeedsConfirmation || res.Resume != "cu_test" || res.ProposedAction == nil || res.ProposedAction.Target != "AXButton 'Delete'" || len(d.actions) != 0 {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
	res2 := e.Resume(context.Background(), "cu_test", true)
	if res2.Status != StatusDone || len(d.actions) != 1 || d.actions[0] != "click:t1" || res2.Steps != 2 {
		t.Fatalf("res2=%+v actions=%v", res2, d.actions)
	}
	if again := e.Resume(context.Background(), "cu_test", true); again.Status != StatusError {
		t.Fatalf("token must be single-use, got %+v", again)
	}
}

func TestRun_TypingIntoSecureFieldNeedsConfirmation(t *testing.T) {
	s := snap("Password")
	s.Elements[0].Role, s.Elements[0].Secure = "AXSecureTextField", true
	d := &FakeDriver{snaps: []Snapshot{s, s}}
	a := &FakeAsker{answers: []map[string]jev.Answer{withInput(decide("type", "e1", 0.95, 0.05, 0), "pw"), decide("done", "none", 0.9, 0, 0.99)}}
	e := newTestEngine(d, a)
	res := e.Run(context.Background(), Request{Goal: "log in", Inputs: map[string]string{"pw": "hunter2"}})
	if res.Status != StatusNeedsConfirmation || res.ProposedAction == nil || res.ProposedAction.InputKey != "pw" || len(d.actions) != 0 {
		t.Fatalf("secure typing must pause: res=%+v actions=%v", res, d.actions)
	}
	if contains(res.Reason, "hunter2") || contains(res.ProposedAction.Target, "hunter2") {
		t.Fatal("input value leaked")
	}
	if res2 := e.Resume(context.Background(), "cu_test", true); res2.Status != StatusDone || d.actions[0] != "type:t1=hunter2" {
		t.Fatalf("resume must type: res2=%+v actions=%v", res2, d.actions)
	}
}

func TestRun_OutOfRangeTargetCountsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e37", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || res.Trace[0].Note != "target_out_of_range" || len(d.actions) != 0 {
		t.Fatalf("res=%+v", res)
	}
}

func TestResume_CancelAndExpiry(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("Delete")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.95, 0.9, 0)}}
	e := newTestEngine(d, a)
	e.Run(context.Background(), Request{Goal: "g"})
	if res := e.Resume(context.Background(), "cu_test", false); res.Status != StatusCancelled {
		t.Fatalf("cancel → %+v", res)
	}
	e.Run(context.Background(), Request{Goal: "g"})
	e.now = func() time.Time { return time.Unix(1_700_000_000+11*60, 0) }
	if res := e.Resume(context.Background(), "cu_test", true); res.Status != StatusError || !contains(res.Reason, "expired") {
		t.Fatalf("expiry → %+v", res)
	}
}

func TestRun_IncompatibleTargetCountsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("OK")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{withInput(decide("type", "e1", 0.9, 0, 0), "x")}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g", Inputs: map[string]string{"x": "v"}})
	if res.Status != StatusStuck || res.Trace[0].Note != "incompatible" || len(d.actions) != 0 {
		t.Fatalf("res=%+v", res)
	}
}

func TestRun_NoChangeThreeTimesIsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || !contains(res.Reason, "no_change") {
		t.Fatalf("res=%+v", res)
	}
}

func TestRun_MaxSteps(t *testing.T) {
	snaps := []Snapshot{}
	for i := 0; i < 10; i++ {
		snaps = append(snaps, snap("A", "B"+string(rune('a'+i))))
	}
	d := &FakeDriver{snaps: snaps}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g", MaxSteps: 4})
	if res.Status != StatusMaxSteps || res.Steps != 4 {
		t.Fatalf("res=%+v", res)
	}
}

func TestRun_DriverUnavailableAndJevError(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}, pingErr: ErrDriverUnavailable}
	res := newTestEngine(d, &FakeAsker{}).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusUnavailable || res.Hint == "" {
		t.Fatalf("res=%+v", res)
	}
	d2 := &FakeDriver{snaps: []Snapshot{snap("A")}}
	res2 := newTestEngine(d2, &FakeAsker{err: &jev.Error{Status: 401, Message: "bad key"}}).Run(context.Background(), Request{Goal: "g"})
	if res2.Status != StatusError || !contains(res2.Reason, "401") {
		t.Fatalf("res2=%+v", res2)
	}
	d3 := &FakeDriver{snaps: []Snapshot{{Degraded: "ax_window_unresolved"}}}
	if res3 := newTestEngine(d3, &FakeAsker{}).Run(context.Background(), Request{Goal: "g"}); res3.Status != StatusStuck || !contains(res3.Reason, "ax_window_unresolved") {
		t.Fatalf("res3=%+v", res3)
	}
	_ = errors.New
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// --- gate coverage the brief's set leaves open -------------------------------

// Rule 1 (done score) and rule 2 (op done) are separate exits; neither may act.
func TestRun_DoneByThresholdAndByOp(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.9, 0, 0.95)}}
	if res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"}); res.Status != StatusDone || res.Steps != 1 || len(d.actions) != 0 {
		t.Fatalf("done score → %+v actions=%v", res, d.actions)
	}
	d2 := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a2 := &FakeAsker{answers: []map[string]jev.Answer{decide("done", "none", 0.9, 0, 0.1)}}
	res2 := newTestEngine(d2, a2).Run(context.Background(), Request{Goal: "g"})
	if res2.Status != StatusDone || res2.Trace[0].Effect != "done" || len(d2.actions) != 0 {
		t.Fatalf("op done → %+v actions=%v", res2, d2.actions)
	}
}

// Rule 3, second half: an op that needs a target but was given "none".
func TestRun_TargetlessActionOpCountsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "none", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || res.Trace[0].Note != "no_action" || len(d.actions) != 0 {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
}

// Rule 5: look narrows the NEXT snapshot by depth, and by the target's label
// when Jev named one. The label is screen content, not an input value.
func TestRun_LookNarrowsNextSnapshot(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("A"), snap("A", "B")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("look", "e1", 0.9, 0, 0), decide("done", "none", 0.9, 0, 0.99)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusDone || len(d.actions) != 0 || res.Trace[0].Effect != "look" {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
	if len(d.opts) < 2 || d.opts[1].MaxDepth != lookMaxDepth || d.opts[1].Query != "A" {
		t.Fatalf("second snapshot opts=%+v", d.opts)
	}
	if d.opts[0].MaxDepth != 0 || d.opts[0].Query != "" {
		t.Fatalf("first snapshot must be unnarrowed: %+v", d.opts[0])
	}
}

// Rule 7: type without an input key never reaches the driver.
func TestRun_TypeWithoutInputKeyCountsStuck(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("Name")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("type", "e1", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || res.Trace[0].Note != "no_input_key" || len(d.actions) != 0 {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
}

// Rule 9: a refused effect is free once, but twice in a row on the same target
// counts toward stuck. The screen changes every step so no_change cannot be
// what ends the run.
func TestRun_RefusedTwiceOnSameTargetCountsStuck(t *testing.T) {
	snaps := []Snapshot{}
	for i := 0; i < 8; i++ {
		snaps = append(snaps, snap("A", "B"+string(rune('a'+i))))
	}
	d := &FakeDriver{snaps: snaps, effect: EffectRefused}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.9, 0, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusStuck || !contains(res.Reason, "stuck:") || res.Steps != 4 || len(d.actions) != 4 {
		t.Fatalf("res=%+v actions=%v", res, d.actions)
	}
	if res.Trace[0].Effect != string(EffectRefused) {
		t.Fatalf("trace[0]=%+v", res.Trace[0])
	}
}

// The paused step must be visible in the trace the caller gets back, so a
// needs_confirmation answer explains itself without a second round trip.
func TestRun_NeedsConfirmationCarriesPendingTraceEntry(t *testing.T) {
	d := &FakeDriver{snaps: []Snapshot{snap("Delete")}}
	a := &FakeAsker{answers: []map[string]jev.Answer{decide("click", "e1", 0.95, 0.8, 0)}}
	res := newTestEngine(d, a).Run(context.Background(), Request{Goal: "g"})
	if res.Status != StatusNeedsConfirmation || len(res.Trace) != 1 || res.Steps != 1 {
		t.Fatalf("res=%+v", res)
	}
	last := res.Trace[0]
	if last.Effect != "pending" || last.Note != "needs_confirmation" || last.Target != res.ProposedAction.Target || last.Risky != 0.8 {
		t.Fatalf("pending trace entry=%+v proposed=%+v", last, res.ProposedAction)
	}
	if len(res.LastScreen) != 1 || res.LastScreen[0] != "[e1] AXButton 'Delete' enabled" {
		t.Fatalf("last_screen=%v", res.LastScreen)
	}
}
