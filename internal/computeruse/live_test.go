//go:build integration

package computeruse

import (
	"context"
	"os"
	"testing"
	"time"
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
