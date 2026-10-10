//go:build e2e

package tarsserver

import "net/http"

// init registers the e2e-only event publisher via e2eRouteHooks. This
// file only compiles with `go build/run -tags e2e` (see
// frontend/console/e2e/webServers.ts); a normal `make build`/`tars serve`
// never includes it, so /v1/e2e/events does not exist outside the
// Playwright harness.
func init() {
	e2eRouteHooks = append(e2eRouteHooks, registerE2EEventsRoute)
}

// registerE2EEventsRoute wires POST /v1/e2e/events: the request body is a
// notificationEvent JSON object, decoded and published straight to the
// broker (not through notificationDispatcher.Emit — this is not a real
// alert, so it must not touch the notification store or desktop
// notifier). It lets an e2e spec make the real server emit a companion
// event (or any other category) on the real /v1/events/stream, the same
// path #1000's initiative sender will use.
func registerE2EEventsRoute(mux *http.ServeMux, broker *eventBroker) {
	if mux == nil || broker == nil {
		return
	}
	mux.HandleFunc("/v1/e2e/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var evt notificationEvent
		if !decodeJSONBody(w, r, &evt) {
			return
		}
		broker.Publish(evt)
		writeJSON(w, http.StatusOK, map[string]bool{"published": true})
	})
}
