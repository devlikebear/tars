package tarsserver

import (
	"net/http"

	"github.com/devlikebear/tars/internal/initiative"
)

// e2eRouteDeps is what an e2e-only route hook may need. It is a struct
// (not a growing parameter list) so a new hook — like tars#1220's
// initiative tick — can read what it needs without re-churning every
// existing hook's signature again.
type e2eRouteDeps struct {
	Broker *eventBroker
	// InitiativeRuntime is nil when initiative is disabled (the common
	// case in most e2e specs); a hook that needs it checks for nil itself.
	InitiativeRuntime *initiative.Runtime
}

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
//
// #1220: the initiative tick hook (e2e_initiative.go) is the second user —
// live mode's actual tick interval (minutes) is far too slow to drive
// deterministically from a spec, and ticking it for real on the shared e2e
// server would risk CASE's bubble appearing mid-spec in an unrelated test
// (the #1194 class of bug CLAUDE.md's verification-scope note warns
// about), so initiative only advances when a spec calls this route.
var e2eRouteHooks []func(mux *http.ServeMux, deps e2eRouteDeps)

// applyE2ERouteHooks runs every registered e2e-only route hook. A plain
// function (not inlined into buildAPIMux) so the hook-invocation mechanism
// itself — empty by default, calling whatever is registered when not —
// has test coverage in a normal build, independent of e2e_events.go's own
// `-tags e2e` content (covered separately, see e2e_events_test.go).
func applyE2ERouteHooks(mux *http.ServeMux, deps e2eRouteDeps) {
	for _, hook := range e2eRouteHooks {
		hook(mux, deps)
	}
}
