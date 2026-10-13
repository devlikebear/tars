package tarsserver

import (
	"net/http"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
	"github.com/rs/zerolog"
)

// newConsolePresenceAPIHandler serves POST /v1/console/presence (tars#1249):
// the console's own signal that a person is actually at the console right
// now, as opposed to merely having something subscribed to
// /v1/events/stream. No request body — the server's own clock is the only
// timestamp that matters, since the console sends no identifying detail at
// all. Not under /v1/admin/, so a plain user token is enough
// (serverauth/middleware.go gates only the admin prefix); every auth mode
// but "off" still requires that token like any other authenticated route.
func newConsolePresenceAPIHandler(store *initiative.PresenceStore, now func() time.Time, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if store == nil || now == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if _, err := store.Record(now()); err != nil {
			// The in-memory state (what the observer reads every tick) is
			// already updated regardless — only the disk write failed, so
			// a restart before the next successful write would lose this
			// one signal. Not worth failing a 204 over; the console will
			// send another one soon.
			logger.Warn().Err(err).Msg("console presence record failed to persist")
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
