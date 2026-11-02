//go:build integration

package computeruse

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/jev"
)

func liveAgentDesktop(t *testing.T) *AgentDesktopDriver {
	t.Helper()
	path, err := FindAgentDesktopPath(os.Getenv(AgentDesktopPathEnv))
	if err != nil {
		t.Skip(err)
	}
	d := NewAgentDesktopDriver(path, 15*time.Second)
	if err := d.Ping(context.Background()); err != nil {
		t.Skipf("agent-desktop not usable: %v", err)
	}
	return d
}

func elementNamed(t *testing.T, snap Snapshot, labels ...string) Element {
	t.Helper()
	for _, el := range snap.Elements {
		for _, label := range labels {
			if el.Label == label {
				return el
			}
		}
	}
	t.Fatalf("none of %q in %d elements", labels, len(snap.Elements))
	return Element{}
}

// Calculator's button names follow the system language, so both are listed.
//
// AGENT_DESKTOP_PATH=<native binary> go test -tags integration ./internal/computeruse -run TestLive_AgentDesktopCalculator -v
func TestLive_AgentDesktopCalculator(t *testing.T) {
	d := liveAgentDesktop(t)
	ctx := context.Background()
	w, err := d.ResolveWindow(ctx, "Calculator")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := d.Snapshot(ctx, w, SnapshotOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var answers []map[string]jev.Answer
	for _, labels := range [][]string{
		{"All Clear", "모두 지우기", "지우기", "Clear"},
		{"2"},
		{"Add", "더하기", "추가", "+"},
		{"3"},
		{"Equals", "같음", "등호", "=", "결과"},
	} {
		el := elementNamed(t, snap, labels...)
		answers = append(answers, decide("click", fmt.Sprintf("e%d", el.Index), 0.95, 0.05, 0))
	}
	answers = append(answers, decide("done", "none", 0.9, 0, 0.99))
	res := NewEngine(d, &FakeAsker{answers: answers}, DefaultConfig()).Run(ctx, Request{Goal: "2+3", App: "Calculator"})
	if res.Status != StatusDone {
		t.Fatalf("res = %+v", res)
	}
	snap, err = d.Snapshot(ctx, w, SnapshotOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(snap.Texts, "\n"), `"5"`) {
		t.Fatalf("texts = %q", snap.Texts)
	}
	if eff, err := d.PressKey(ctx, w, "escape"); err != nil {
		t.Fatalf("press: %v %v", eff, err)
	}
}

// CU_SECURE_APP names a running app with a secure field labelled by
// CU_SECURE_LABEL. Use a test app: a fake value is written into that field.
func TestLive_AgentDesktopSecureField(t *testing.T) {
	app, label := os.Getenv("CU_SECURE_APP"), os.Getenv("CU_SECURE_LABEL")
	if app == "" || label == "" {
		t.Skip("set CU_SECURE_APP and CU_SECURE_LABEL")
	}
	d := liveAgentDesktop(t)
	ctx := context.Background()
	w, err := d.ResolveWindow(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := d.Snapshot(ctx, w, SnapshotOpts{MaxDepth: lookMaxDepth})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("partial=%v elements=%d", snap.Partial, len(snap.Elements))
	el := elementNamed(t, snap, label)
	if !el.Secure || el.Value != "" {
		t.Fatalf("element = %+v", el)
	}
	const value = "-fixture-only-value"
	target := fmt.Sprintf("e%d", el.Index)
	a := &FakeAsker{answers: []map[string]jev.Answer{
		withInput(decide("type", target, 0.95, 0.05, 0), "pw"),
		decide("done", "none", 0.9, 0, 0.99),
	}}
	e := NewEngine(d, a, DefaultConfig())
	res := e.Run(ctx, Request{Goal: "fill", App: app, Inputs: map[string]string{"pw": value}})
	if res.Status != StatusNeedsConfirmation {
		t.Fatalf("res = %+v", res)
	}
	res = e.Resume(ctx, res.Resume, true)
	if res.Status != StatusDone {
		t.Fatalf("resume = %+v", res)
	}
	if got := res.Trace[0].Effect; got != string(EffectUnverifiable) {
		t.Fatalf("effect = %q", got)
	}
	if eff, err := d.Scroll(ctx, w, "down"); err != nil {
		t.Fatalf("scroll: %v %v", eff, err)
	}
}
