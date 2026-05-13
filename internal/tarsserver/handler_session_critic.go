package tarsserver

import (
	"errors"
	"net/http"

	"github.com/devlikebear/tars/internal/session"
)

// handleSessionCritic serves /v1/admin/sessions/{id}/critic — GET, PUT, DELETE.
// PUT body: {"enabled": bool, "max_iterations": N?}. DELETE clears the entire
// critic configuration. GET returns {"critic": SessionCritic|null}. All
// session kinds may carry a critic; worker/subagent sessions typically inherit
// from their parent at creation time but the API stays open per session.
func handleSessionCritic(w http.ResponseWriter, r *http.Request, reqStore *session.Store, sessionID string) {
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
		writeJSON(w, http.StatusOK, map[string]any{"critic": sess.Critic})

	case http.MethodPut:
		var req struct {
			Enabled       bool `json:"enabled"`
			MaxIterations int  `json:"max_iterations,omitempty"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		critic := &session.SessionCritic{
			Enabled:       req.Enabled,
			MaxIterations: req.MaxIterations,
			Status:        session.SessionCriticStatusIdle,
		}
		updated, err := reqStore.SetCritic(sessionID, critic)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"critic": updated.Critic})

	case http.MethodDelete:
		updated, err := reqStore.SetCritic(sessionID, nil)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"critic": updated.Critic})
	}
}
