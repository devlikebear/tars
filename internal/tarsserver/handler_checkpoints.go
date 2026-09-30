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
//	GET  /v1/admin/sessions/{id}/checkpoints/{turn}/diff    ?scope=turn|session|since&path=
//	POST /v1/admin/sessions/{id}/checkpoints/{turn}/revert  checkpoint.RevertRequest
//	POST /v1/admin/sessions/{id}/checkpoints/reverts/{revert}/undo  {"force": bool}
//
// A revert or undo answers 409 when a turn is running (turn_in_progress) or
// when later edits conflict and force is off (revert_conflict, with the
// result so the conflicts can be shown).
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
		reverts, err := store.Reverts(sessionID)
		if err != nil {
			writeCheckpointError(w, logger, err)
			return
		}
		if turns == nil {
			turns = []checkpoint.Entry{}
		}
		if reverts == nil {
			reverts = []checkpoint.RevertEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "turns": turns, "reverts": reverts})
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
	mux.HandleFunc("POST /v1/admin/sessions/{id}/checkpoints/{turn}/revert", func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := checkpointSession(w, r, store, sessions)
		if !ok {
			return
		}
		var req checkpoint.RevertRequest
		if !decodeOptionalJSONBody(w, r, &req) {
			return
		}
		result, err := store.Revert(r.Context(), sessionID, r.PathValue("turn"), req)
		writeRevertResult(w, logger, result, err)
	})
	mux.HandleFunc("POST /v1/admin/sessions/{id}/checkpoints/reverts/{revert}/undo", func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := checkpointSession(w, r, store, sessions)
		if !ok {
			return
		}
		var req struct {
			Force bool `json:"force"`
		}
		if !decodeOptionalJSONBody(w, r, &req) {
			return
		}
		result, err := store.Undo(r.Context(), sessionID, r.PathValue("revert"), req.Force)
		writeRevertResult(w, logger, result, err)
	})
	return mux
}

func writeRevertResult(w http.ResponseWriter, logger zerolog.Logger, result checkpoint.RevertResult, err error) {
	if result.Files == nil {
		result.Files = []checkpoint.RevertFileResult{}
	}
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, checkpoint.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"code": "revert_conflict", "error": err.Error(), "result": result})
	case errors.Is(err, checkpoint.ErrBusy):
		writeError(w, http.StatusConflict, "turn_in_progress", err.Error())
	default:
		writeCheckpointError(w, logger, err)
	}
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
