package tarsserver

import (
	"context"
	"net/http"

	"github.com/devlikebear/tars/internal/reflection"
	"github.com/rs/zerolog"
)

// reflectionRuntimeAPI is the narrow interface the reflection HTTP
// handler requires. The real *reflection.Runtime satisfies it; tests
// pass a fake.
type reflectionRuntimeAPI interface {
	RunOnce(ctx context.Context) reflection.RunSummary
	Snapshot() reflection.Snapshot
}

// reflectionConfigView mirrors the operator-relevant subset of
// reflection.Config exposed at /v1/reflection/config.
type reflectionConfigView struct {
	Enabled                bool   `json:"enabled"`
	SleepWindow            string `json:"sleep_window"`
	Timezone               string `json:"timezone"`
	TickIntervalSeconds    int    `json:"tick_interval_seconds"`
	EmptySessionAgeSeconds int64  `json:"empty_session_age_seconds"`
	MemoryLookbackHours    int    `json:"memory_lookback_hours"`
	MaxTurnsPerSession     int    `json:"max_turns_per_session"`
}

// newReflectionAPIHandler serves /v1/reflection/* endpoints.
//
// A nil runtime yields handlers that return empty status / 503 on
// run-once. The config view is always served from cfg, so even a
// disabled reflection reports its policy for inspection.
func newReflectionAPIHandler(runtime reflectionRuntimeAPI, cfg reflectionConfigView, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/reflection/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, reflection.Snapshot{})
			return
		}
		writeJSON(w, http.StatusOK, runtime.Snapshot())
	})

	mux.HandleFunc("/v1/reflection/run-once", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "reflection runtime is not configured",
			})
			return
		}
		summary := runtime.RunOnce(r.Context())
		if !summary.Success && summary.Err != "" {
			logger.Warn().Str("err", summary.Err).Msg("reflection run-once reported error")
		}
		writeJSON(w, http.StatusOK, summary)
	})

	mux.HandleFunc("/v1/reflection/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	})

	return mux
}
