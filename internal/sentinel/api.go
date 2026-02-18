package sentinel

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func NewAPIHandler(supervisor *Supervisor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sentinel/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if supervisor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sentinel supervisor is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, supervisor.Status())
	})
	mux.HandleFunc("/v1/sentinel/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if supervisor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sentinel supervisor is not configured"})
			return
		}
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = v
		}
		events := supervisor.Events(limit)
		writeJSON(w, http.StatusOK, map[string]any{"count": len(events), "events": events})
	})
	mux.HandleFunc("/v1/sentinel/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if supervisor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sentinel supervisor is not configured"})
			return
		}
		status, err := supervisor.Restart(context.Background())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/v1/sentinel/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if supervisor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sentinel supervisor is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, supervisor.Pause())
	})
	mux.HandleFunc("/v1/sentinel/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if supervisor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sentinel supervisor is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, supervisor.Resume())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
