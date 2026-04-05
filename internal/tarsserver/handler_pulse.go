package tarsserver

import (
	"context"
	"net/http"

	"github.com/devlikebear/tars/internal/pulse"
	"github.com/rs/zerolog"
)

// pulseRuntimeAPI is the narrow interface the pulse HTTP handler needs.
// The real *pulse.Runtime satisfies it; tests can pass a fake.
type pulseRuntimeAPI interface {
	RunOnce(ctx context.Context) pulse.TickOutcome
	Snapshot() pulse.Snapshot
}

// pulseConfigView is the read-only config shape exposed at /v1/pulse/config.
// It mirrors the fields an operator would want to inspect without
// revealing derived or internal values.
type pulseConfigView struct {
	Enabled              bool     `json:"enabled"`
	IntervalSeconds      int      `json:"interval_seconds"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	ActiveHours          string   `json:"active_hours"`
	Timezone             string   `json:"timezone"`
	MinSeverity          string   `json:"min_severity"`
	AllowedAutofixes     []string `json:"allowed_autofixes"`
	NotifyTelegram       bool     `json:"notify_telegram"`
	NotifySessionEvents  bool     `json:"notify_session_events"`
	CronFailureThreshold int      `json:"cron_failure_threshold"`
	StuckRunMinutes      int      `json:"stuck_run_minutes"`
	DiskWarnPercent      float64  `json:"disk_warn_percent"`
	DiskCriticalPercent  float64  `json:"disk_critical_percent"`
}

// newPulseAPIHandler returns an http.Handler serving /v1/pulse/* endpoints.
// A nil runtime yields handlers that return empty/default responses — this
// lets the server start even when pulse is disabled in config.
func newPulseAPIHandler(runtime pulseRuntimeAPI, cfg pulseConfigView, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/pulse/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, pulse.Snapshot{})
			return
		}
		writeJSON(w, http.StatusOK, runtime.Snapshot())
	})

	mux.HandleFunc("/v1/pulse/run-once", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "pulse runtime is not configured",
			})
			return
		}
		outcome := runtime.RunOnce(r.Context())
		if outcome.Err != "" {
			logger.Warn().Str("err", outcome.Err).Msg("pulse run-once reported error")
		}
		writeJSON(w, http.StatusOK, outcome)
	})

	mux.HandleFunc("/v1/pulse/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	})

	return mux
}
