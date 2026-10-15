package tarsserver

import "testing"

func TestConsoleBuildState_GetSet(t *testing.T) {
	state := newConsoleBuildState("initial")
	if got := state.Get(); got != "initial" {
		t.Fatalf("expected initial value, got %q", got)
	}

	state.Set("updated")
	if got := state.Get(); got != "updated" {
		t.Fatalf("expected updated value, got %q", got)
	}
}

func TestConsoleBuildState_NilIsSafe(t *testing.T) {
	var state *consoleBuildState
	if got := state.Get(); got != "" {
		t.Fatalf("expected empty value from a nil state, got %q", got)
	}
	// Must not panic.
	state.Set("ignored")
}
