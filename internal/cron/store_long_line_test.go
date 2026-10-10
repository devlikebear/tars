package cron

import (
	"os"
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

func TestLoadRunsSkipsBlankLinesAndFailsOnCorruptOnes(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.appendRunRecord(RunRecord{JobID: "job-1", Response: "ok"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	path := runPath(store.runsDir, "job-1")
	appendRaw := func(raw string) {
		t.Helper()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(raw); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	appendRaw("\n   \n" + `{"job_id":"job-1","response":"no newline"}`)
	runs, err := store.loadRuns("job-1")
	if err != nil || len(runs) != 2 || runs[1].Response != "no newline" {
		t.Fatalf("runs = %d, err=%v", len(runs), err)
	}

	appendRaw("\n{not json\n")
	if _, err := store.loadRuns("job-1"); err == nil || !strings.Contains(err.Error(), "decode cron run") {
		t.Fatalf("err = %v, want a decode error", err)
	}
}
