package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/rs/zerolog"
)

func newBrowserAPIHandler(runtime *gateway.Runtime, vaultStatus vaultStatusSnapshot, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/browser/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, runtime.BrowserStatus())
	})
	mux.HandleFunc("/v1/browser/profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		profiles := runtime.BrowserProfiles()
		writeJSON(w, http.StatusOK, map[string]any{"count": len(profiles), "profiles": profiles})
	})
	mux.HandleFunc("/v1/browser/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		var req struct {
			SiteID  string `json:"site_id"`
			Profile string `json:"profile,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		res, err := runtime.BrowserLogin(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.Profile))
		if err != nil {
			logger.Warn().Err(err).Str("site_id", req.SiteID).Msg("browser login failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/v1/browser/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		var req struct {
			SiteID  string `json:"site_id"`
			Profile string `json:"profile,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		res, err := runtime.BrowserCheck(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.Profile))
		if err != nil {
			logger.Warn().Err(err).Str("site_id", req.SiteID).Msg("browser check failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/v1/browser/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		var req struct {
			SiteID     string `json:"site_id"`
			FlowAction string `json:"flow_action"`
			Profile    string `json:"profile,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		res, err := runtime.BrowserRun(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.FlowAction), strings.TrimSpace(req.Profile))
		if err != nil {
			logger.Warn().Err(err).Str("site_id", req.SiteID).Str("flow_action", req.FlowAction).Msg("browser run failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/v1/vault/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, vaultStatus)
	})
	return mux
}
