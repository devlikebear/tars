package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/ops"
	"github.com/rs/zerolog"
)

func newOpsAPIHandler(manager *ops.Manager, logger zerolog.Logger, emit func(context.Context, notificationEvent)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/ops/status", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status, err := manager.Status(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("ops status failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops status failed"})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})

	mux.HandleFunc("/v1/ops/cleanup/plan", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		plan, err := manager.CreateCleanupPlan(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("ops cleanup plan failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops cleanup plan failed"})
			return
		}
		if emit != nil {
			emit(r.Context(), newNotificationEvent("ops", "warn", "Cleanup approval required", "approval_id="+plan.ApprovalID))
		}
		writeJSON(w, http.StatusOK, plan)
	})

	mux.HandleFunc("/v1/ops/cleanup/apply", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ApprovalID string `json:"approval_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		result, err := manager.ApplyCleanup(r.Context(), strings.TrimSpace(req.ApprovalID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if emit != nil {
			emit(r.Context(), newNotificationEvent("ops", "info", "Cleanup applied", "approval_id="+result.ApprovalID))
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/v1/ops/approvals", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, err := manager.ListApprovals()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list approvals failed"})
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	mux.HandleFunc("/v1/ops/approvals/", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/ops/approvals/")
		parts := strings.Split(path, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			http.NotFound(w, r)
			return
		}
		approvalID := strings.TrimSpace(parts[0])
		action := strings.TrimSpace(parts[1])
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var err error
		switch action {
		case "approve":
			err = manager.Approve(approvalID)
			if err == nil && emit != nil {
				emit(r.Context(), newNotificationEvent("ops", "info", "Cleanup approval approved", "approval_id="+approvalID))
			}
		case "reject":
			err = manager.Reject(approvalID)
			if err == nil && emit != nil {
				emit(r.Context(), newNotificationEvent("ops", "warn", "Cleanup approval rejected", "approval_id="+approvalID))
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approval_id": approvalID, "action": action, "ok": true})
	})

	return mux
}
