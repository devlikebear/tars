package tarsserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
)

// handleSessionGoal serves /v1/admin/sessions/{id}/goal — GET, PUT, DELETE.
// PUT body: {"description": "...", "max_auto_continues": N?, "permission_mode": "auto"?}
// DELETE clears the goal. GET returns {"goal": SessionGoal|null}.
//
// permission_mode is the tool permission mode approved together with the
// goal: it becomes the session's mode while the goal is active and the
// previous mode comes back when the goal ends (see session.SessionGoal).
// PUT and DELETE also return the session's resulting "permission_mode".
// Only user chats (the main session and ordinary chats) may carry a goal;
// worker and subagent sessions return 400.
func handleSessionGoal(w http.ResponseWriter, r *http.Request, reqStore *session.Store, sessionID string, audit func(ops.AutomationAuditEntry)) {
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
			PermissionMode   string `json:"permission_mode,omitempty"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		mode := strings.TrimSpace(req.PermissionMode)
		if mode != "" && !validChatPermissionMode(mode) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "permission_mode must be manual, accept_edits, plan, auto, or empty"})
			return
		}
		before := sessionPermissionMode(reqStore, sessionID)
		goal := &session.SessionGoal{
			Description:      req.Description,
			MaxAutoContinues: req.MaxAutoContinues,
			Status:           session.SessionGoalStatusActive,
			PermissionMode:   mode,
		}
		updated, err := reqStore.SetGoal(sessionID, goal)
		if err != nil {
			switch {
			case errors.Is(err, session.ErrSessionNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			case errors.Is(err, session.ErrSessionKindUnsupported):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only chat sessions support goals"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		auditGoalPermissionMode(audit, sessionID, before, updated.PermissionMode, "goal_set")
		writeJSON(w, http.StatusOK, map[string]any{"goal": updated.Goal, "permission_mode": updated.PermissionMode})

	case http.MethodDelete:
		before := sessionPermissionMode(reqStore, sessionID)
		updated, err := reqStore.ClearGoal(sessionID)
		if err != nil {
			if errors.Is(err, session.ErrSessionNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		auditGoalPermissionMode(audit, sessionID, before, updated.PermissionMode, "goal_cleared")
		writeJSON(w, http.StatusOK, map[string]any{"goal": updated.Goal, "permission_mode": updated.PermissionMode})
	}
}

func sessionPermissionMode(store *session.Store, sessionID string) string {
	sess, err := store.Get(sessionID)
	if err != nil {
		return ""
	}
	return sess.PermissionMode
}

// auditGoalPermissionMode records a permission mode change that came with a
// goal (granted when it was set, handed back when it ended) in the same
// audit action as a direct mode change.
func auditGoalPermissionMode(audit func(ops.AutomationAuditEntry), sessionID, from, to, reason string) {
	if audit == nil || from == to {
		return
	}
	audit(ops.AutomationAuditEntry{
		Timestamp: time.Now().UTC(),
		Actor:     "console",
		Action:    "chat_permission_mode",
		SessionID: sessionID,
		Result:    "changed",
		Details:   map[string]any{"from": from, "to": to, "reason": reason},
	})
}
