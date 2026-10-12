package tarsserver

import (
	"net/http"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
)

// initiativeStatusResponse is initiative.Runtime.Snapshot() plus the
// console presence facts (tars#1249) — timestamps only, never message text
// or a session id (#1003's rule).
type initiativeStatusResponse struct {
	initiative.Snapshot
	Presence initiativeStatusPresence `json:"presence"`
}

type initiativeStatusPresence struct {
	LastAt    *time.Time `json:"last_at,omitempty"`
	ArrivedAt *time.Time `json:"arrived_at,omitempty"`
	Present   bool       `json:"present"`
}

func initiativeStatusPresenceFrom(state initiative.PresenceState, now time.Time) initiativeStatusPresence {
	out := initiativeStatusPresence{Present: state.Present(now)}
	if !state.LastAt.IsZero() {
		lastAt := state.LastAt
		out.LastAt = &lastAt
	}
	if !state.ArrivedAt.IsZero() {
		arrivedAt := state.ArrivedAt
		out.ArrivedAt = &arrivedAt
	}
	return out
}

// newInitiativeAPIHandler serves GET /v1/initiative/status. presence and now
// may be nil (setup-only mode, or initiative disabled entirely) — the
// response then always reports presence.present=false with no timestamps,
// same as before this field existed.
func newInitiativeAPIHandler(runtime *initiative.Runtime, presence *initiative.PresenceStore, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/initiative/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		resp := initiativeStatusResponse{Snapshot: runtime.Snapshot()}
		if presence != nil && now != nil {
			resp.Presence = initiativeStatusPresenceFrom(presence.Current(), now())
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux
}
