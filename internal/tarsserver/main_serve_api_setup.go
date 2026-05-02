package tarsserver

import (
	"net/http"
)

// setupOnlyHandlers groups the small set of HTTP handlers that the
// degraded boot mode (Phase 2 of the onboarding plan) wires into a
// dedicated mux. Anything outside this set is intentionally absent —
// chat / agent runtime / cron / pulse / reflection / memory / sessions
// have no useful response without an LLM router and the catch-all
// 503 fallback steers the wizard toward the supported surface.
type setupOnlyHandlers struct {
	healthz http.Handler
	setup   http.Handler
	config  http.Handler // serves /v1/admin/config{,/values,/schema} and /v1/admin/restart
	auth    http.Handler
	events  http.Handler // optional — SSE keepalive so the SPA stays connected
	console http.Handler
}

// registerSetupOnlyRoutes wires the minimal set of endpoints the
// onboarding wizard needs onto mux. All other /v1/* paths get the
// setupOnlyFallback (HTTP 503 + JSON hint), and "/" redirects to the
// console so a browser hitting the root lands on the wizard.
func registerSetupOnlyRoutes(mux *http.ServeMux, handlers setupOnlyHandlers) {
	mux.Handle("/v1/healthz", handlers.healthz)
	mux.Handle("/v1/setup/status", handlers.setup)
	mux.Handle("/v1/admin/config", handlers.config)
	mux.Handle("/v1/admin/config/values", handlers.config)
	mux.Handle("/v1/admin/config/schema", handlers.config)
	mux.Handle("/v1/admin/restart", handlers.config)
	mux.Handle("/v1/auth/whoami", handlers.auth)
	if handlers.events != nil {
		mux.Handle("/v1/events/stream", handlers.events)
	}
	if handlers.console != nil {
		mux.Handle("/console", handlers.console)
		mux.Handle("/console/", handlers.console)
	}
	mux.Handle("/v1/", http.HandlerFunc(setupOnlyFallback))
	mux.Handle("/", http.HandlerFunc(setupOnlyRoot))
}

// setupOnlyFallback rejects unsupported /v1/* requests with a 503 +
// JSON hint pointing at /console. The Phase 3 wizard should never
// hit this path in practice, but a clear response beats a confusing
// 404 if it does (or if external code probes the server).
func setupOnlyFallback(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":       "service unavailable in setup-only mode",
		"hint":        "complete setup at /console to enable this endpoint",
		"path":        r.URL.Path,
		"needs_setup": true,
	})
}

// setupOnlyRoot mirrors the production root handler: "/" redirects to
// the console wizard, anything else 404s. Console assets themselves
// are served by the dedicated /console handler registered above.
func setupOnlyRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/console/", http.StatusFound)
		return
	}
	http.NotFound(w, r)
}
