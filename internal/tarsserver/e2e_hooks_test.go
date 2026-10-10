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
	var gotDeps e2eRouteDeps
	calls := 0
	e2eRouteHooks = []func(*http.ServeMux, e2eRouteDeps){
		func(mux *http.ServeMux, deps e2eRouteDeps) {
			calls++
			gotMux = mux
			gotDeps = deps
		},
	}

	mux := http.NewServeMux()
	deps := e2eRouteDeps{Broker: newEventBroker()}
	applyE2ERouteHooks(mux, deps)

	if calls != 1 {
		t.Fatalf("expected the registered hook to run once, got %d calls", calls)
	}
	if gotMux != mux || gotDeps.Broker != deps.Broker {
		t.Fatalf("expected the hook to receive the same mux/deps passed in")
	}
}

// TestApplyE2ERouteHooks_NoopWhenEmpty is the default-build case: nothing
// registered, nothing runs, no panic on a nil/empty slice.
func TestApplyE2ERouteHooks_NoopWhenEmpty(t *testing.T) {
	original := e2eRouteHooks
	e2eRouteHooks = nil
	t.Cleanup(func() { e2eRouteHooks = original })

	applyE2ERouteHooks(http.NewServeMux(), e2eRouteDeps{Broker: newEventBroker()})
}
