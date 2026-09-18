//go:build integration

package computeruse

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/jev"
)

func liveDriver(t *testing.T) *CuaDriver {
	t.Helper()
	path, err := FindCuaDriverPath(os.Getenv(CuaDriverPathEnv))
	if err != nil {
		t.Skip(err)
	}
	d := NewCuaDriver(path, 15*time.Second)
	if err := d.Ping(context.Background()); err != nil {
		t.Skipf("cua-driver daemon not reachable: %v", err)
	}
	return d
}

// go test -tags integration ./internal/computeruse -run TestLive_Snapshot -v
func TestLive_Snapshot(t *testing.T) {
	d := liveDriver(t)
	ctx := context.Background()
	w, err := d.ResolveWindow(ctx, os.Getenv("CU_APP")) // e.g. CU_APP=Finder
	if err != nil {
		t.Fatal(err)
	}
	snap, err := d.Snapshot(ctx, w, SnapshotOpts{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("window=%+v elements=%d total=%d degraded=%q", w, len(snap.Elements), snap.TotalElements, snap.Degraded)
	for i, el := range snap.Elements {
		if i >= 25 {
			break
		}
		t.Logf("[e%d] %s %q value=%q enabled=%v secure=%v", el.Index, el.Role, el.Label, el.Value, el.Enabled, el.Secure)
	}
	if len(snap.Elements) == 0 && snap.Degraded == "" {
		t.Fatal("no elements and not degraded")
	}
}

// CU_APP=Calculator go test -tags integration ./internal/computeruse -run TestLive_Loop -v
func TestLive_Loop(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set")
	}
	d := liveDriver(t)
	goal := os.Getenv("CU_GOAL")
	if goal == "" {
		goal = "Press the buttons 2, +, 3, = so the display shows 5"
	}
	cfg := DefaultConfig()
	cfg.MaxSteps = 12
	e := NewEngine(d, jev.NewClient(jev.Config{APIKey: key}), cfg)
	res := e.Run(context.Background(), Request{Goal: goal, App: os.Getenv("CU_APP")})
	for _, ts := range res.Trace {
		t.Logf("step %d: %s %s conf=%.2f risky=%.2f done=%.2f → %s %s (%dms, %d tok)", ts.Step, ts.Op, ts.Target, ts.Confidence, ts.Risky, ts.Done, ts.Effect, ts.Note, ts.LatencyMS, ts.InputTokens)
	}
	t.Logf("status=%s reason=%s usage=%+v", res.Status, res.Reason, res.Usage)
	if res.Status != StatusDone {
		t.Fatalf("expected done, got %s: %s", res.Status, res.Reason)
	}
}
