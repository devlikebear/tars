package usage

import (
	"testing"
	"time"
)

func TestTracker_RecordSignalAndSummarize(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	if err := tracker.RecordSignal(SignalEntry{
		Name:      "tool_call",
		Source:    "chat",
		SessionID: "sess-1",
		Dimensions: map[string]string{
			"tool":   "process",
			"action": "poll",
		},
	}); err != nil {
		t.Fatalf("record signal: %v", err)
	}
	if err := tracker.RecordSignal(SignalEntry{
		Name:      "tool_call",
		Source:    "chat",
		SessionID: "sess-1",
		Count:     2,
		Dimensions: map[string]string{
			"tool":   "process",
			"action": "poll",
		},
	}); err != nil {
		t.Fatalf("record second signal: %v", err)
	}

	summary, err := tracker.Signals("today")
	if err != nil {
		t.Fatalf("signals: %v", err)
	}
	if summary.TotalCount != 3 {
		t.Fatalf("expected total count 3, got %+v", summary)
	}
	if len(summary.Rows) != 1 {
		t.Fatalf("expected one row, got %+v", summary.Rows)
	}
	row := summary.Rows[0]
	if row.Name != "tool_call" || row.Count != 3 || row.Dimensions["tool"] != "process" || row.Dimensions["action"] != "poll" {
		t.Fatalf("unexpected row: %+v", row)
	}
}
