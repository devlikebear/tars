package pulse

import (
	"testing"
	"time"
)

func TestNewStateDefaultsCapacity(t *testing.T) {
	s := NewState(0)
	if s.cap != 50 {
		t.Errorf("default cap = %d, want 50", s.cap)
	}
	s2 := NewState(-1)
	if s2.cap != 50 {
		t.Errorf("negative cap = %d, want 50", s2.cap)
	}
}

func TestStateRecordTickUpdatesCounters(t *testing.T) {
	s := NewState(10)
	base := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)

	s.RecordTick(TickOutcome{At: base, Skipped: true, SkipReason: "out_of_hours"})
	s.RecordTick(TickOutcome{
		At:             base.Add(time.Minute),
		DeciderInvoked: true,
		Decision:       &Decision{Action: ActionNotify, Severity: SeverityWarn, Title: "disk"},
	})
	s.RecordTick(TickOutcome{
		At:             base.Add(2 * time.Minute),
		DeciderInvoked: true,
		Decision:       &Decision{Action: ActionAutofix, AutofixName: "compress_old_logs"},
		AutofixAttempt: "compress_old_logs",
		AutofixOK:      true,
	})

	snap := s.Snapshot()
	if snap.TotalTicks != 3 {
		t.Errorf("TotalTicks = %d, want 3", snap.TotalTicks)
	}
	if snap.TotalSkipped != 1 {
		t.Errorf("TotalSkipped = %d, want 1", snap.TotalSkipped)
	}
	if snap.TotalDecisions != 2 {
		t.Errorf("TotalDecisions = %d, want 2", snap.TotalDecisions)
	}
	if snap.TotalNotifies != 1 {
		t.Errorf("TotalNotifies = %d, want 1", snap.TotalNotifies)
	}
	if snap.TotalAutofixes != 1 {
		t.Errorf("TotalAutofixes = %d, want 1", snap.TotalAutofixes)
	}
	if snap.LastDecision == nil || snap.LastDecision.Action != ActionAutofix {
		t.Errorf("LastDecision = %+v, want Autofix", snap.LastDecision)
	}
	if snap.LastTickAt != base.Add(2*time.Minute) {
		t.Errorf("LastTickAt = %v, want %v", snap.LastTickAt, base.Add(2*time.Minute))
	}
}

func TestStateSnapshotReturnsChronological(t *testing.T) {
	s := NewState(3)
	base := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		s.RecordTick(TickOutcome{At: base.Add(time.Duration(i) * time.Minute)})
	}
	snap := s.Snapshot()
	// Ring cap 3, so we keep the last 3: minutes 2, 3, 4 in order.
	if len(snap.Recent) != 3 {
		t.Fatalf("Recent len = %d, want 3", len(snap.Recent))
	}
	for i, want := range []int{2, 3, 4} {
		got := snap.Recent[i].At
		if got != base.Add(time.Duration(want)*time.Minute) {
			t.Errorf("Recent[%d] = %v, want minute %d", i, got, want)
		}
	}
}

func TestStateSnapshotDecouplesFromInternalState(t *testing.T) {
	s := NewState(5)
	s.RecordTick(TickOutcome{Decision: &Decision{Action: ActionNotify, Title: "first"}})
	snap := s.Snapshot()
	if snap.LastDecision == nil {
		t.Fatal("LastDecision nil")
	}
	snap.LastDecision.Title = "mutated"
	snap2 := s.Snapshot()
	if snap2.LastDecision.Title != "first" {
		t.Errorf("snapshot mutation leaked: got %q", snap2.LastDecision.Title)
	}
}

func TestStateNilReceiverSafe(t *testing.T) {
	var s *State
	s.RecordTick(TickOutcome{}) // must not panic
	snap := s.Snapshot()
	if snap.TotalTicks != 0 {
		t.Errorf("nil snapshot TotalTicks = %d, want 0", snap.TotalTicks)
	}
}

func TestStateRecordTickAutoFillsAt(t *testing.T) {
	s := NewState(1)
	before := time.Now()
	s.RecordTick(TickOutcome{}) // no At → should auto-fill
	after := time.Now()
	snap := s.Snapshot()
	if snap.LastTickAt.Before(before) || snap.LastTickAt.After(after) {
		t.Errorf("auto-filled At %v not in [%v, %v]", snap.LastTickAt, before, after)
	}
}
