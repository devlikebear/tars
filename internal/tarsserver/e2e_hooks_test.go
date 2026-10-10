package tarsserver

import (
	"net/http"
	"testing"
)

// TestE2ERouteHooks_EmptyWithoutE2ETag locks in that a normal build (this
// test file itself has no `e2e` build tag, so it runs in `make test`)
// never registers the e2e-only event publisher: e2e_events.go's init only
// compiles with `-tags e2e` (#1192). If this starts failing, something is
// appending to e2eRouteHooks outside that build-tagged file.
func TestE2ERouteHooks_EmptyWithoutE2ETag(t *testing.T) {
	if len(e2eRouteHooks) != 0 {
		t.Fatalf("expected no e2e route hooks in a default build, got %d", len(e2eRouteHooks))
	}
}

// TestApplyE2ERouteHooks_RunsEveryRegisteredHook covers the hook-invocation
// mechanism itself in a normal build (no `-tags e2e` needed): whatever is
// registered in e2eRouteHooks runs with the given mux/broker. The route
// e2e_events.go actually registers is e2e-tag-only and has its own
// coverage there (e2e_events_test.go); this is the generic plumbing that
// buildAPIMux calls unconditionally.
func TestApplyE2ERouteHooks_RunsEveryRegisteredHook(t *testing.T) {
	original := e2eRouteHooks
	t.Cleanup(func() { e2eRouteHooks = original })

	var gotMux *http.ServeMux
	var gotBroker *eventBroker
	calls := 0
	e2eRouteHooks = []func(*http.ServeMux, *eventBroker){
		func(mux *http.ServeMux, broker *eventBroker) {
			calls++
			gotMux = mux
			gotBroker = broker
		},
	}

	mux := http.NewServeMux()
	broker := newEventBroker()
	applyE2ERouteHooks(mux, broker)

	if calls != 1 {
		t.Fatalf("expected the registered hook to run once, got %d calls", calls)
	}
	if gotMux != mux || gotBroker != broker {
		t.Fatalf("expected the hook to receive the same mux/broker passed in")
	}
}

// TestApplyE2ERouteHooks_NoopWhenEmpty is the default-build case: nothing
// registered, nothing runs, no panic on a nil/empty slice.
func TestApplyE2ERouteHooks_NoopWhenEmpty(t *testing.T) {
	original := e2eRouteHooks
	e2eRouteHooks = nil
	t.Cleanup(func() { e2eRouteHooks = original })

	applyE2ERouteHooks(http.NewServeMux(), newEventBroker())
}
