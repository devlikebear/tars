package initiative

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPresenceStoreThrottlesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presence.json")
	store := OpenPresenceStore(path)
	now := at(9, 0)

	// First signal: a new arrival always writes.
	if _, err := store.Record(now); err != nil {
		t.Fatalf("Record: %v", err)
	}
	st, err := LoadPresenceState(path)
	if err != nil || !st.LastAt.Equal(now) {
		t.Fatalf("after first write: state=%+v err=%v, want LastAt=%v", st, err, now)
	}

	// A continuing signal 30s later must not write again yet (throttled to
	// once a minute), even though Current() reflects it in memory.
	now2 := now.Add(30 * time.Second)
	if _, err := store.Record(now2); err != nil {
		t.Fatalf("Record: %v", err)
	}
	st, err = LoadPresenceState(path)
	if err != nil || !st.LastAt.Equal(now) {
		t.Fatalf("after throttled write: state=%+v err=%v, want unchanged at %v", st, err, now)
	}
	if cur := store.Current(); !cur.LastAt.Equal(now2) {
		t.Fatalf("Current() = %+v, want in-memory LastAt=%v even though disk is throttled", cur, now2)
	}

	// Past a minute since the last write, the next signal writes again.
	now3 := now.Add(70 * time.Second)
	if _, err := store.Record(now3); err != nil {
		t.Fatalf("Record: %v", err)
	}
	st, err = LoadPresenceState(path)
	if err != nil || !st.LastAt.Equal(now3) {
		t.Fatalf("after the throttle window elapses: state=%+v err=%v, want LastAt=%v", st, err, now3)
	}
}

func TestPresenceStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presence.json")
	now := at(9, 0)
	a := OpenPresenceStore(path)
	if _, err := a.Record(now); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// A fresh store (simulating a server restart) reads the same state
	// back from disk rather than starting as "never arrived".
	b := OpenPresenceStore(path)
	if cur := b.Current(); !cur.LastAt.Equal(now) || !cur.ArrivedAt.Equal(now) {
		t.Fatalf("restarted store = %+v, want state restored from disk", cur)
	}
}

func TestMissingPresenceFileReadsAsZeroState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "presence.json")
	st, err := LoadPresenceState(path)
	if err != nil {
		t.Fatalf("LoadPresenceState: %v", err)
	}
	if !st.LastAt.IsZero() || !st.ArrivedAt.IsZero() {
		t.Fatalf("state = %+v, want zero", st)
	}
}

func TestRecordPresenceArrivalDetection(t *testing.T) {
	now := at(9, 0)
	// First signal ever is an arrival.
	s := RecordPresence(PresenceState{}, now)
	if !s.LastAt.Equal(now) || !s.ArrivedAt.Equal(now) {
		t.Fatalf("first signal = %+v, want LastAt=ArrivedAt=%v", s, now)
	}

	// A signal 2 minutes later continues the same arrival.
	s2 := RecordPresence(s, now.Add(2*time.Minute))
	if !s2.LastAt.Equal(now.Add(2*time.Minute)) || !s2.ArrivedAt.Equal(now) {
		t.Fatalf("continued signal = %+v, want LastAt advanced, ArrivedAt unchanged", s2)
	}

	// A gap of exactly presenceTTL or more starts a new arrival.
	s3 := RecordPresence(s2, s2.LastAt.Add(presenceTTL))
	want3 := s2.LastAt.Add(presenceTTL)
	if !s3.LastAt.Equal(want3) || !s3.ArrivedAt.Equal(want3) {
		t.Fatalf("signal after a %v gap = %+v, want a fresh arrival at %v", presenceTTL, s3, want3)
	}

	// A gap just under presenceTTL still continues the arrival.
	s4 := RecordPresence(s2, s2.LastAt.Add(presenceTTL-time.Second))
	if !s4.ArrivedAt.Equal(s2.ArrivedAt) {
		t.Fatalf("signal just under the gap = %+v, want ArrivedAt unchanged", s4)
	}
}

func TestRecordPresenceIgnoresOutOfOrderSignal(t *testing.T) {
	now := at(9, 0)
	s := RecordPresence(PresenceState{}, now)
	// An e2e spec can drive synthetic timestamps out of order across
	// specs sharing a server; a signal older than the last one recorded
	// must never move presence backward.
	stale := RecordPresence(s, now.Add(-time.Hour))
	if stale != s {
		t.Fatalf("out-of-order signal changed state: got %+v, want unchanged %+v", stale, s)
	}
}

// TestOvernightPresenceGapThenMorningSignalIsArrival is the issue's own
// scenario (tars#1249): a presence signal stops overnight (the SSE
// subscription itself may well have stayed up the whole time — that no
// longer matters) and a fresh signal the next morning, far past the last
// user message, must read as JustArrived.
func TestOvernightPresenceGapThenMorningSignalIsArrival(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	lastNight := at(20, 0)
	morning := lastNight.Add(11 * time.Hour) // next day 07:00
	obs := Observation{
		Now:              morning,
		ConsolePresentAt: morning,
		ConsoleArrivedAt: morning,
		LastUserAt:       lastNight,
	}
	g := deriveGoSignals(cfg, obs, History{})
	if !g.JustArrived {
		t.Fatalf("signals = %+v, want JustArrived after an overnight presence gap", g)
	}
	decision := Decide(g, TextSignals{})
	if decision.Intent != IntentGreet {
		t.Fatalf("decision = %+v, want a greet", decision)
	}
}

// TestStaleSubscriptionAloneNeverDelivers is the issue's other half: a
// console tab can stay subscribed to /v1/events/stream for hours with
// nobody there. Without a presence signal at all, the console must not be
// picked as a delivery channel — this is the deliverLocked-level check,
// complementing the deriveGoSignals-level one in signals_test.go.
func TestStaleSubscriptionAloneNeverDelivers(t *testing.T) {
	now := at(9, 0)
	obs := Observation{Now: now} // no ConsolePresentAt at all
	if consolePresent(obs) {
		t.Fatalf("consolePresent(%+v) = true, want false with no presence signal", obs)
	}
}
