package tarsserver

import (
	"errors"
	"net/http"

	"github.com/devlikebear/tars/internal/session"
)

// handleSessionGoal serves /v1/admin/sessions/{id}/goal — GET, PUT, DELETE.
// PUT body: {"description": "...", "max_auto_continues": N?}
// DELETE clears the goal. GET returns {"goal": SessionGoal|null}.
// Only main-kind sessions are allowed to carry a goal; others return 400.
func handleSessionGoal(w http.ResponseWriter, r *http.Request, reqStore *session.Store, sessionID string) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodDelete) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		sess, err := reqStore.Get(sessionID)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"goal": sess.Goal})

	case http.MethodPut:
		var req struct {
			Description      string `json:"description"`
			MaxAutoContinues int    `json:"max_auto_continues,omitempty"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		goal := &session.SessionGoal{
			Description:      req.Description,
			MaxAutoContinues: req.MaxAutoContinues,
			Status:           session.SessionGoalStatusActive,
		}
		updated, err := reqStore.SetGoal(sessionID, goal)
		if err != nil {
			switch {
			case errors.Is(err, session.ErrSessionNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			case errors.Is(err, session.ErrSessionKindUnsupported):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only main sessions support goals"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"goal": updated.Goal})

	case http.MethodDelete:
		updated, err := reqStore.ClearGoal(sessionID)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"goal": updated.Goal})
	}
}
