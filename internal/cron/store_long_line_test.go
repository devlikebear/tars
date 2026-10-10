package cron

import (
	"strings"
	"testing"
	"time"
)

// A run's response is a whole model reply; one over bufio.Scanner's 64KB
// line limit must not make the job's history unreadable (#1231).
func TestLoadRunsReadsRecordOverScannerLimit(t *testing.T) {
	store := NewStore(t.TempDir())
	long := strings.Repeat("r", 300*1024)
	ranAt := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	for _, rec := range []RunRecord{
		{JobID: "job-1", RanAt: ranAt, Response: long},
		{JobID: "job-1", RanAt: ranAt.Add(time.Minute), Response: "short"},
	} {
		if err := store.appendRunRecord(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	runs, err := store.loadRuns("job-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(runs) != 2 || runs[0].Response != long || runs[1].Response != "short" {
		t.Fatalf("loaded %d runs", len(runs))
	}
}
