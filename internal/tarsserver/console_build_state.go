package tarsserver

import "sync"

// consoleBuildState holds the console_build value healthz reports
// (tars#1257): normally the embedded console bundle's index.html hash,
// fixed for the server process's lifetime. The e2e-only hook
// (e2e_console_build.go) can override it so a Playwright spec can
// simulate a new server build landing — and the console noticing it —
// without restarting the process. A pointer (not a plain string) because
// the real healthz handler and the e2e route both need to see the same
// mutable value.
type consoleBuildState struct {
	mu    sync.RWMutex
	value string
}

func newConsoleBuildState(value string) *consoleBuildState {
	return &consoleBuildState{value: value}
}

// Get returns the current value, or "" for a nil state (no console
// handler configured, e.g. some tests).
func (s *consoleBuildState) Get() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Set overrides the current value. A no-op on a nil state.
func (s *consoleBuildState) Set(value string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
}
