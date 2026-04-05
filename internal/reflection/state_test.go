package reflection

import (
	"testing"
	"time"
)

func TestNewStateDefaultsCapacity(t *testing.T) {
	s := NewState(0)
	if s.cap != 14 {
		t.Errorf("default cap = %d, want 14", s.cap)
	}
	if s2 := NewState(-1); s2.cap != 14 {
		t.Errorf("negative cap = %d, want 14", s2.cap)
	}
}

func TestStateRecordSuccessfulRun(t *testing.T) {
	s := NewState(5)
	now := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)
	s.RecordRun(RunSummary{
		StartedAt:  now,
		FinishedAt: now.Add(time.Minute),
		Success:    true,
		Results:    []JobResult{{Name: "memory", Success: true, Changed: true}},
	})

	snap := s.Snapshot()
	if snap.TotalRuns != 1 || snap.TotalSuccesses != 1 || snap.TotalFailures != 0 {
		t.Fatalf("counters wrong: %+v", snap)
	}
	if snap.ConsecutiveFailures != 0 {
		t.Error("successful run should reset consecutive failures")
	}
	if !snap.LastRunSuccess {
		t.Error("LastRunSuccess should be true")
	}
	if snap.LastSuccessfulRunAt.IsZero() {
		t.Error("LastSuccessfulRunAt should be set")
	}
}

func TestStateConsecutiveFailures(t *testing.T) {
	s := NewState(5)
	base := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		s.RecordRun(RunSummary{
			StartedAt:  base.Add(time.Duration(i) * 24 * time.Hour),
			FinishedAt: base.Add(time.Duration(i) * 24 * time.Hour).Add(time.Minute),
			Success:    false,
		})
	}
	if n := s.ConsecutiveFailures(); n != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", n)
	}
	// Successful run resets.
	s.RecordRun(RunSummary{StartedAt: base.Add(4 * 24 * time.Hour), FinishedAt: base.Add(4*24*time.Hour + time.Minute), Success: true})
	if n := s.ConsecutiveFailures(); n != 0 {
		t.Errorf("after success ConsecutiveFailures = %d, want 0", n)
	}
}

func TestStateRingBufferOverwrites(t *testing.T) {
	s := NewState(3)
	base := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		s.RecordRun(RunSummary{
			StartedAt:  base.Add(time.Duration(i) * time.Hour),
			FinishedAt: base.Add(time.Duration(i)*time.Hour + time.Minute),
			Success:    true,
		})
	}
	snap := s.Snapshot()
	if len(snap.Recent) != 3 {
		t.Fatalf("Recent len = %d, want 3", len(snap.Recent))
	}
	// Oldest surviving entry should be run index 2 (0 and 1 evicted).
	want := base.Add(2 * time.Hour)
	if !snap.Recent[0].StartedAt.Equal(want) {
		t.Errorf("Recent[0].StartedAt = %v, want %v", snap.Recent[0].StartedAt, want)
	}
}

func TestStateMarkAttemptStarted(t *testing.T) {
	s := NewState(3)
	now := time.Date(2026, 4, 5, 2, 30, 0, 0, time.UTC)
	s.MarkAttemptStarted(now)
	if got := s.LastRunAt(); !got.Equal(now) {
		t.Errorf("LastRunAt = %v, want %v", got, now)
	}
	// Earlier time should not move back.
	s.MarkAttemptStarted(now.Add(-1 * time.Hour))
	if got := s.LastRunAt(); !got.Equal(now) {
		t.Errorf("LastRunAt moved backwards: %v", got)
	}
}

func TestStateNilReceiverSafe(t *testing.T) {
	var s *State
	s.RecordRun(RunSummary{})
	s.MarkAttemptStarted(time.Now())
	if n := s.ConsecutiveFailures(); n != 0 {
		t.Errorf("nil ConsecutiveFailures = %d, want 0", n)
	}
	if !s.LastRunAt().IsZero() {
		t.Error("nil LastRunAt should be zero")
	}
	if snap := s.Snapshot(); snap.TotalRuns != 0 {
		t.Errorf("nil Snapshot TotalRuns = %d, want 0", snap.TotalRuns)
	}
}

func TestConfigDefaults(t *testing.T) {
	var c Config
	if c.EffectiveTickInterval() != 5*time.Minute {
		t.Error("default tick != 5m")
	}
	if c.EffectiveSleepWindow() != "02:00-05:00" {
		t.Error("default window")
	}
	if c.EffectiveEmptySessionAge() != 24*time.Hour {
		t.Error("default empty age != 24h")
	}
	if c.EffectiveMemoryLookback() != 24*time.Hour {
		t.Error("default lookback != 24h")
	}
	if c.EffectiveMaxTurnsPerSession() != 20 {
		t.Error("default max turns != 20")
	}
}

func TestSeverityString(t *testing.T) {
	for _, c := range []struct {
		s    Severity
		want string
	}{
		{SeverityInfo, "info"},
		{SeverityWarn, "warn"},
		{SeverityError, "error"},
	} {
		if got := c.s.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", int(c.s), got, c.want)
		}
	}
}
