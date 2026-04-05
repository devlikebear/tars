package pulse

import (
	"sync"
	"time"
)

// State is the in-memory runtime state of a Pulse Runtime. It tracks the
// last tick, the last decision, and a ring buffer of recent tick outcomes
// for observability.
//
// State is safe for concurrent use. The API surface is intentionally
// narrow: writers (the runtime) call RecordTick with a complete outcome,
// and readers (the HTTP handler and tests) retrieve snapshots.
type State struct {
	mu             sync.RWMutex
	cap            int
	lastTickAt     time.Time
	lastDecision   *Decision
	lastErr        string
	totalTicks     int
	totalSkipped   int
	totalDecisions int
	totalAutofixes int
	totalNotifies  int
	recent         []TickOutcome // ring buffer, len == cap once full
	head           int
	size           int
}

// NewState creates a state with the given ring buffer capacity. A non-
// positive capacity falls back to 50 entries.
func NewState(capacity int) *State {
	if capacity <= 0 {
		capacity = 50
	}
	return &State{
		cap:    capacity,
		recent: make([]TickOutcome, capacity),
	}
}

// RecordTick appends an outcome, updating counters and last-* fields.
func (s *State) RecordTick(outcome TickOutcome) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalTicks++
	if outcome.At.IsZero() {
		outcome.At = time.Now()
	}
	s.lastTickAt = outcome.At

	if outcome.Skipped {
		s.totalSkipped++
	}
	if outcome.DeciderInvoked {
		s.totalDecisions++
	}
	if outcome.Decision != nil {
		// Store a copy so later mutation by caller doesn't race.
		dc := *outcome.Decision
		s.lastDecision = &dc
		switch outcome.Decision.Action {
		case ActionNotify:
			s.totalNotifies++
		case ActionAutofix:
			s.totalAutofixes++
		}
	}
	if outcome.Err != "" {
		s.lastErr = outcome.Err
	}

	s.recent[s.head] = outcome
	s.head = (s.head + 1) % s.cap
	if s.size < s.cap {
		s.size++
	}
}

// Snapshot is a point-in-time view of state safe to serialize to JSON.
type Snapshot struct {
	LastTickAt     time.Time     `json:"last_tick_at"`
	LastDecision   *Decision     `json:"last_decision,omitempty"`
	LastErr        string        `json:"last_err,omitempty"`
	TotalTicks     int           `json:"total_ticks"`
	TotalSkipped   int           `json:"total_skipped"`
	TotalDecisions int           `json:"total_decisions"`
	TotalAutofixes int           `json:"total_autofixes"`
	TotalNotifies  int           `json:"total_notifies"`
	Recent         []TickOutcome `json:"recent"`
}

// Snapshot returns a copy of the current state.
func (s *State) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		LastTickAt:     s.lastTickAt,
		LastErr:        s.lastErr,
		TotalTicks:     s.totalTicks,
		TotalSkipped:   s.totalSkipped,
		TotalDecisions: s.totalDecisions,
		TotalAutofixes: s.totalAutofixes,
		TotalNotifies:  s.totalNotifies,
	}
	if s.lastDecision != nil {
		dc := *s.lastDecision
		snap.LastDecision = &dc
	}
	// Reconstruct recent in chronological order (oldest → newest).
	snap.Recent = make([]TickOutcome, 0, s.size)
	start := (s.head - s.size + s.cap) % s.cap
	for i := 0; i < s.size; i++ {
		snap.Recent = append(snap.Recent, s.recent[(start+i)%s.cap])
	}
	return snap
}
