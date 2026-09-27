package tarsserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// newCheckpointAPIHandler serves a session's turn checkpoints (#969):
//
//	GET /v1/admin/sessions/{id}/checkpoints              the recorded turns, oldest first
//	GET /v1/admin/sessions/{id}/checkpoints/{turn}/diff  ?scope=turn|session|since&path=
//
// It is mounted on those patterns ahead of the session handler's prefix.
func newCheckpointAPIHandler(store *checkpoint.Store, sessions *session.Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/admin/sessions/{id}/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := checkpointSession(w, r, store, sessions)
		if !ok {
			return
		}
		turns, err := store.List(sessionID)
		if err != nil {
			writeCheckpointError(w, logger, err)
			return
		}
		if turns == nil {
			turns = []checkpoint.Entry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "turns": turns})
	})
	mux.HandleFunc("GET /v1/admin/sessions/{id}/checkpoints/{turn}/diff", func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := checkpointSession(w, r, store, sessions)
		if !ok {
			return
		}
		query := r.URL.Query()
		scope := checkpoint.Scope(strings.TrimSpace(query.Get("scope")))
		result, err := store.Diff(r.Context(), sessionID, r.PathValue("turn"), scope, strings.TrimSpace(query.Get("path")))
		if err != nil {
			writeCheckpointError(w, logger, err)
			return
		}
		if result.Files == nil {
			result.Files = []checkpoint.FileDiff{}
		}
		writeJSON(w, http.StatusOK, result)
	})
	return mux
}

// checkpointSession resolves the path's session, answering the request itself
// when checkpoints are off or the session does not exist.
func checkpointSession(w http.ResponseWriter, r *http.Request, store *checkpoint.Store, sessions *session.Store) (string, bool) {
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "checkpoints_unavailable", "checkpoints are unavailable (is git installed?)")
		return "", false
	}
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessions != nil {
		if _, err := sessions.Get(sessionID); err != nil {
			if isSessionNotFound(err) {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return "", false
			}
			writeError(w, http.StatusInternalServerError, "", err.Error())
			return "", false
		}
	}
	return sessionID, true
}

func writeCheckpointError(w http.ResponseWriter, logger zerolog.Logger, err error) {
	switch {
	case errors.Is(err, checkpoint.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "turn has no checkpoint")
	case errors.Is(err, checkpoint.ErrSkipped):
		writeError(w, http.StatusConflict, "checkpoint_skipped", err.Error())
	case errors.Is(err, checkpoint.ErrInvalid):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		logger.Warn().Err(err).Msg("checkpoint request failed")
		writeError(w, http.StatusInternalServerError, "", err.Error())
	}
}
