package reflection

import (
	"sync"
	"time"
)

// State is the in-memory runtime state of a reflection Runtime. It tracks
// the last run, a short history of recent runs, and counters pulse reads
// via the ReflectionHealthSource interface.
//
// State is safe for concurrent use. The exposed Snapshot is a deep-ish
// copy suitable for JSON serialization without holding the internal lock.
type State struct {
	mu                  sync.RWMutex
	cap                 int
	lastRunAt           time.Time
	lastRunSuccess      bool
	lastRunSummary      *RunSummary
	lastSuccessfulRunAt time.Time
	consecutiveFailures int
	totalRuns           int
	totalSuccesses      int
	totalFailures       int
	recent              []RunSummary // ring buffer
	head                int
	size                int
}

// NewState creates a reflection state with the given ring buffer capacity.
// A non-positive capacity falls back to 14 (roughly two weeks of nightly
// runs).
func NewState(capacity int) *State {
	if capacity <= 0 {
		capacity = 14
	}
	return &State{
		cap:    capacity,
		recent: make([]RunSummary, capacity),
	}
}

// RecordRun appends a summary and updates counters. Called by the runtime
// at the end of each reflection run.
func (s *State) RecordRun(summary RunSummary) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalRuns++
	s.lastRunAt = summary.FinishedAt
	if s.lastRunAt.IsZero() {
		s.lastRunAt = time.Now()
	}
	copied := summary
	s.lastRunSummary = &copied
	s.lastRunSuccess = summary.Success

	if summary.Success {
		s.totalSuccesses++
		s.consecutiveFailures = 0
		s.lastSuccessfulRunAt = s.lastRunAt
	} else {
		s.totalFailures++
		s.consecutiveFailures++
	}

	s.recent[s.head] = summary
	s.head = (s.head + 1) % s.cap
	if s.size < s.cap {
		s.size++
	}
}

// MarkAttemptStarted lets the scheduler tag "today" as attempted even
// before jobs finish, so a crash mid-run doesn't cause the next tick to
// immediately retry the same day. The returned boolean indicates whether
// this is the first attempt today.
func (s *State) MarkAttemptStarted(now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// No counters bumped here — RecordRun is the authoritative counter
	// update. MarkAttemptStarted is only used to influence "already ran
	// today" decisions via lastRunAt.
	if now.IsZero() {
		now = time.Now()
	}
	if s.lastRunAt.Before(now) {
		// Conservatively push lastRunAt forward so the scheduler sees
		// that we attempted today. If the run later succeeds RecordRun
		// will overwrite with the precise finish time.
		s.lastRunAt = now
	}
}

// Snapshot is a point-in-time view of state safe to serialize.
type Snapshot struct {
	LastRunAt           time.Time    `json:"last_run_at"`
	LastRunSuccess      bool         `json:"last_run_success"`
	LastRunSummary      *RunSummary  `json:"last_run_summary,omitempty"`
	LastSuccessfulRunAt time.Time    `json:"last_successful_run_at,omitempty"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	TotalRuns           int          `json:"total_runs"`
	TotalSuccesses      int          `json:"total_successes"`
	TotalFailures       int          `json:"total_failures"`
	Recent              []RunSummary `json:"recent"`
}

// Snapshot returns a chronologically ordered copy of current state.
func (s *State) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		LastRunAt:           s.lastRunAt,
		LastRunSuccess:      s.lastRunSuccess,
		LastSuccessfulRunAt: s.lastSuccessfulRunAt,
		ConsecutiveFailures: s.consecutiveFailures,
		TotalRuns:           s.totalRuns,
		TotalSuccesses:      s.totalSuccesses,
		TotalFailures:       s.totalFailures,
	}
	if s.lastRunSummary != nil {
		summary := *s.lastRunSummary
		snap.LastRunSummary = &summary
	}
	snap.Recent = make([]RunSummary, 0, s.size)
	start := (s.head - s.size + s.cap) % s.cap
	for i := 0; i < s.size; i++ {
		snap.Recent = append(snap.Recent, s.recent[(start+i)%s.cap])
	}
	return snap
}

// ConsecutiveFailures implements pulse.ReflectionHealthSource — pulse's
// signal scanner reads this to decide whether to emit a reflection-
// failure signal.
func (s *State) ConsecutiveFailures() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consecutiveFailures
}

// LastRunAt implements pulse.ReflectionHealthSource.
func (s *State) LastRunAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRunAt
}
