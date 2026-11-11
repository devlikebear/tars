package initiative

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// briefCaptureEvery throttles how often a run of presence signals takes a
// new snapshot: one capture reads every focus pipeline, the chat activity
// and the cron jobs, and the console sends a signal up to every two
// minutes of real input.
const briefCaptureEvery = time.Minute

// briefFile is snapshot.json: the last snapshot taken while the user was at
// the console (Seen), the one frozen when they came back after an absence
// (Away, what a greet compares against), and the arrival that froze it.
type briefFile struct {
	ArrivedAt time.Time      `json:"arrived_at,omitempty"`
	Seen      StatusSnapshot `json:"seen"`
	Away      StatusSnapshot `json:"away"`
}

// BriefStore keeps the snapshots an arrival brief compares. The server
// hands every console presence signal to Seen: while the user is at the
// console it keeps Seen current (throttled), and the first signal of a new
// arrival (RecordPresence started a new run) freezes the last Seen as Away
// before taking a new one. So Away is TARS as it was the last time the user
// was seen before the gap, which is the moment they left — taken from
// their own last signal rather than from the tick that later notices the
// silence, ten minutes after they left.
//
// The file lives next to the ledger and holds pipeline titles and cron job
// names: it is state, like the reminder and result queues, not the ledger,
// which still never holds a title or an id.
type BriefStore struct {
	path    string
	capture func(now time.Time) StatusSnapshot

	mu          sync.Mutex
	loaded      bool
	state       briefFile
	lastCapture time.Time
}

// OpenBriefStore prepares a store backed by path. capture reads TARS's
// state now; it is called with no lock of the caller held.
func OpenBriefStore(path string, capture func(now time.Time) StatusSnapshot) *BriefStore {
	return &BriefStore{path: path, capture: capture}
}

// Seen folds one presence signal (the state PresenceStore.Record returned
// for it) into the store.
func (s *BriefStore) Seen(presence PresenceState, now time.Time) error {
	if s == nil || s.capture == nil || presence.ArrivedAt.IsZero() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	arrival := !presence.ArrivedAt.Equal(s.state.ArrivedAt)
	if !arrival && !s.lastCapture.IsZero() && now.Sub(s.lastCapture) < briefCaptureEvery && !now.Before(s.lastCapture) {
		return nil
	}
	if arrival {
		s.state.Away = s.state.Seen
		s.state.ArrivedAt = presence.ArrivedAt
	}
	snap := s.capture(now)
	snap.At = now
	s.state.Seen = snap
	s.lastCapture = now
	return s.saveLocked()
}

// Lines is the arrival brief for now in both console languages: the same
// items in the same order, English and Korean words. Empty when no
// snapshot was frozen at the current arrival or nothing changed since.
func (s *BriefStore) Lines(now time.Time) (en, ko []BriefItem) {
	if s == nil || s.capture == nil {
		return nil, nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	away := s.state.Away
	s.mu.Unlock()
	if away.At.IsZero() {
		return nil, nil
	}
	current := s.capture(now)
	current.At = now
	return Brief(away, current, "en"), Brief(away, current, "ko")
}

func (s *BriefStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var state briefFile
	if json.Unmarshal(raw, &state) == nil {
		s.state = state
	}
}

func (s *BriefStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
