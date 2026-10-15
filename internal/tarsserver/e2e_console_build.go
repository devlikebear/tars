//go:build e2e

package tarsserver

import "net/http"

// init registers the e2e-only console_build override route via
// e2eRouteHooks. This file only compiles with `go build/run -tags e2e`
// (see frontend/console/e2e/webServers.ts); a normal `make build`/`tars
// serve` never includes it, so /v1/e2e/console-build does not exist
// outside the Playwright harness.
func init() {
	e2eRouteHooks = append(e2eRouteHooks, registerE2EConsoleBuildRoute)
}

type e2eConsoleBuildRequest struct {
	// Build is the new value healthz should report as console_build.
	// Empty is valid — it simulates the dev-proxy/unbuilt case where the
	// field is omitted from the response.
	Build string `json:"build"`
}

// registerE2EConsoleBuildRoute wires POST /v1/e2e/console-build: a spec
// simulates the server shipping a new console bundle (tars#1257) without
// restarting the shared e2e server — real console_build changes come
// from a server restart, which specs cannot do mid-run without disrupting
// other tests on the same process (the #1194 class of bug the
// e2e-route-hook mechanism exists to avoid; see e2e_hooks.go).
func registerE2EConsoleBuildRoute(mux *http.ServeMux, deps e2eRouteDeps) {
	if mux == nil || deps.ConsoleBuild == nil {
		return
	}
	mux.HandleFunc("/v1/e2e/console-build", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req e2eConsoleBuildRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		deps.ConsoleBuild.Set(req.Build)
		writeJSON(w, http.StatusOK, map[string]string{"console_build": req.Build})
	})
}
