package initiative

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// presenceTTL is how stale a presence signal (POST /v1/console/presence,
// tars#1249) may be before the console reads as "nobody at the desk"
// again. Chosen to match arrivalFreshness and the console's own
// COMPANION_JUST_ARRIVED_HIDDEN_MS (10 minutes) so "arrived"/"present" mean
// the same gap on both sides: the console sends a signal on open and every
// 2 minutes of real input after that, so five missed signals in a row (10
// minutes of silence) is a reasonable line between "briefly between
// keystrokes" and "actually gone".
const presenceTTL = 10 * time.Minute

// PresenceState is the small, identifier-free record a presence signal
// updates: when it last arrived, and when the current unbroken run of
// signals began. Persisted by the server as the entire content of
// presence.json (tars#1249) — no message text, no session id. tars doctor
// reads the same file directly (LoadPresenceState) to report the same two
// facts without a running server.
type PresenceState struct {
	LastAt    time.Time `json:"last_at,omitempty"`
	ArrivedAt time.Time `json:"arrived_at,omitempty"`
}

// Present reports whether s's last signal is still fresh as of now — the
// same freshness rule consolePresent applies to an Observation, exported so
// the status handler and tars doctor can report "at the console right now"
// without duplicating the TTL.
func (s PresenceState) Present(now time.Time) bool {
	return !s.LastAt.IsZero() && now.Sub(s.LastAt) <= presenceTTL
}

// RecordPresence folds one new presence signal into prev. A signal older
// than the last one recorded is ignored outright (out-of-order delivery, or
// an e2e spec driving synthetic timestamps): presence time never moves
// backward. Otherwise, a signal arriving within presenceTTL of the last one
// continues the same arrival (ArrivedAt unchanged); a gap of presenceTTL or
// more — or no prior signal at all — starts a new one (ArrivedAt = now).
func RecordPresence(prev PresenceState, now time.Time) PresenceState {
	if !prev.LastAt.IsZero() && now.Before(prev.LastAt) {
		return prev
	}
	if prev.LastAt.IsZero() || now.Sub(prev.LastAt) >= presenceTTL {
		return PresenceState{LastAt: now, ArrivedAt: now}
	}
	return PresenceState{LastAt: now, ArrivedAt: prev.ArrivedAt}
}

// consolePresent reports whether obs's last recorded presence signal is
// still fresh as of obs.Now — the one check every other console-presence
// signal (Reachable, IdleAtDesk, JustArrived's consoleUp gate, the delivery
// channel choice) goes through, so they all agree on what "present" means.
func consolePresent(obs Observation) bool {
	return PresenceState{LastAt: obs.ConsolePresentAt}.Present(obs.Now)
}

// PresenceStore persists PresenceState to a small JSON file — tars#1249's
// entire durable record of console presence, with no message text or
// session id. Writes are throttled (tars#1249's cost note): a new arrival
// always writes, a continuing one writes at most once a minute, so a
// console sending a signal every 2 minutes of real input never exceeds one
// write per minute — the same "don't put a model, or in this case extra
// disk I/O, on a per-minute path for free" discipline pulse's cost incident
// established for the LLM decider.
type PresenceStore struct {
	path string

	mu        sync.Mutex
	loaded    bool
	state     PresenceState
	lastSaved time.Time
}

// OpenPresenceStore prepares a store backed by path. The file is read lazily
// on first use, not here, so constructing one never fails or blocks.
func OpenPresenceStore(path string) *PresenceStore {
	return &PresenceStore{path: path}
}

// Record folds one new presence signal (now, the server's own clock — the
// console sends no timestamp) into the store and returns the resulting
// state. Safe for concurrent use.
func (s *PresenceStore) Record(now time.Time) (PresenceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	prevArrivedAt := s.state.ArrivedAt
	s.state = RecordPresence(s.state, now)
	newArrival := !s.state.ArrivedAt.Equal(prevArrivedAt)
	if !newArrival && !s.lastSaved.IsZero() && now.Sub(s.lastSaved) < time.Minute {
		return s.state, nil
	}
	if err := s.saveLocked(); err != nil {
		return s.state, err
	}
	s.lastSaved = now
	return s.state, nil
}

// Current returns the last recorded state without writing a new signal —
// what the status handler and the observer read every tick.
func (s *PresenceStore) Current() PresenceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.state
}

func (s *PresenceStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	if state, err := LoadPresenceState(s.path); err == nil {
		s.state = state
	}
}

func (s *PresenceStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}

// LoadPresenceState reads a presence.json file directly, without a
// PresenceStore — tars doctor runs standalone and has no long-lived server
// state to hold one. A missing file reads as the zero state (no signal
// ever recorded), not an error.
func LoadPresenceState(path string) (PresenceState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PresenceState{}, nil
		}
		return PresenceState{}, err
	}
	var state PresenceState
	if err := json.Unmarshal(raw, &state); err != nil {
		return PresenceState{}, err
	}
	return state, nil
}
