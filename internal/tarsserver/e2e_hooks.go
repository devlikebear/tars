package tarsserver

import "net/http"

// e2eRouteHooks lets a file built only with `-tags e2e` (see
// e2e_events.go) register extra, test-only routes on the real server mux
// without the production binary knowing anything about them. Empty by
// default, so a normal build never exposes a test-only endpoint — the
// production API surface is exactly what registerAPIRoutes wires up.
//
// #1192: Playwright specs need a way to make the real server emit an
// arbitrary notificationEvent (a companion expression/line/session_id) on
// the real /v1/events/stream, the same path a real sender (initiative,
// #1000) will use later, rather than faking the SSE response in the
// browser. This is that door, kept out of the shipped binary.
var e2eRouteHooks []func(mux *http.ServeMux, broker *eventBroker)

// applyE2ERouteHooks runs every registered e2e-only route hook. A plain
// function (not inlined into buildAPIMux) so the hook-invocation mechanism
// itself — empty by default, calling whatever is registered when not —
// has test coverage in a normal build, independent of e2e_events.go's own
// `-tags e2e` content (covered separately, see e2e_events_test.go).
func applyE2ERouteHooks(mux *http.ServeMux, broker *eventBroker) {
	for _, hook := range e2eRouteHooks {
		hook(mux, broker)
	}
}
