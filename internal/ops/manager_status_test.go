package ops

import (
	"context"
	"testing"
)

// TestManager_StatusReportsDiskAndProcessCounts exercises the platform-split
// diskUsage helper, which had no test on either platform. Both implementations
// report the space available to this process, so the only portable assertions
// are the invariants: a non-empty volume, free space that cannot exceed it, and
// a percentage in range.
func TestManager_StatusReportsDiskAndProcessCounts(t *testing.T) {
	mgr := NewManager(t.TempDir(), Options{HomeDir: t.TempDir()})

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.DiskTotalBytes == 0 {
		t.Fatal("expected a non-zero total disk size")
	}
	if status.DiskFreeBytes > status.DiskTotalBytes {
		t.Fatalf("free %d exceeds total %d", status.DiskFreeBytes, status.DiskTotalBytes)
	}
	if status.DiskUsedPercent < 0 || status.DiskUsedPercent > 100 {
		t.Fatalf("used percent %.2f is out of range", status.DiskUsedPercent)
	}
	if status.ProcessCount <= 0 {
		t.Fatalf("expected a positive process count, got %d", status.ProcessCount)
	}
	if status.Timestamp.IsZero() {
		t.Fatal("expected a timestamp")
	}
}

func TestManager_StatusRejectsNilManager(t *testing.T) {
	var mgr *Manager
	if _, err := mgr.Status(context.Background()); err == nil {
		t.Fatal("expected an error from a nil manager")
	}
}
