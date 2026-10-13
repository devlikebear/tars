//go:build e2e

package tarsserver

import (
	"net/http"
	"time"
)

// init registers the e2e-only initiative tick route via e2eRouteHooks. This
// file only compiles with `go build/run -tags e2e`; a normal `make
// build`/`tars serve` never includes it, so /v1/e2e/initiative/tick does
// not exist outside the Playwright harness.
func init() {
	e2eRouteHooks = append(e2eRouteHooks, registerE2EInitiativeTickRoute)
}

type e2eInitiativeTickRequest struct {
	// At is the RFC3339 timestamp the tick runs as "now" (Runtime.RunOnceAt,
	// tars#1220). Missing/empty uses the real wall clock.
	At string `json:"at"`
	// PresenceAt is zero or more RFC3339 timestamps (tars#1249) recorded
	// into the presence tracker, in order, before the tick runs — a
	// spec's stand-in for the console actually sending POST
	// /v1/console/presence at those moments. Each one is folded through
	// initiative.RecordPresence exactly like a real signal (a gap of
	// presenceTTL or more starts a new arrival), so a spec drives arrival
	// detection the same way the real endpoint does, just without a
	// browser tab open at that synthetic time.
	PresenceAt []string `json:"presence_at,omitempty"`
}

// registerE2EInitiativeTickRoute wires POST /v1/e2e/initiative/tick: a
// spec-driven tick, the only way initiative ever advances on the e2e
// server (initiative.tick is set to a long interval there specifically so
// the real ticker never fires mid-spec — see e2e_hooks.go's #1220 note).
// 404 when initiative is disabled or not in live mode, matching the real
// status handler's "no runtime" shape rather than panicking.
func registerE2EInitiativeTickRoute(mux *http.ServeMux, deps e2eRouteDeps) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/v1/e2e/initiative/tick", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if deps.InitiativeRuntime == nil {
			writeError(w, http.StatusNotFound, "", "initiative is not running")
			return
		}
		var req e2eInitiativeTickRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		now := time.Now()
		if at := req.At; at != "" {
			parsed, err := time.Parse(time.RFC3339, at)
			if err != nil {
				writeError(w, http.StatusBadRequest, "", "at must be RFC3339")
				return
			}
			now = parsed
		}
		if deps.Presence != nil {
			for _, raw := range req.PresenceAt {
				parsed, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					writeError(w, http.StatusBadRequest, "", "presence_at must be RFC3339")
					return
				}
				if _, err := deps.Presence.Record(parsed); err != nil {
					writeError(w, http.StatusInternalServerError, "", "record presence failed")
					return
				}
			}
		}
		entry := deps.InitiativeRuntime.RunOnceAt(r.Context(), now)
		writeJSON(w, http.StatusOK, entry)
	})
}
