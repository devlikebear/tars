package browserplugin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/plugin"
)

func (p *Plugin) HTTPHandlers() []plugin.HTTPHandlerEntry {
	if p.service == nil {
		return nil
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/browser/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSONResponse(w, http.StatusOK, p.service.Status())
	})

	mux.HandleFunc("/v1/browser/profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		profiles := p.service.Profiles()
		writeJSONResponse(w, http.StatusOK, map[string]any{"count": len(profiles), "profiles": profiles})
	})

	mux.HandleFunc("/v1/browser/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SiteID  string `json:"site_id"`
			Profile string `json:"profile,omitempty"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		res, err := p.service.Login(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.Profile))
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSONResponse(w, http.StatusOK, res)
	})

	mux.HandleFunc("/v1/browser/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SiteID  string `json:"site_id"`
			Profile string `json:"profile,omitempty"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		res, err := p.service.Check(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.Profile))
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSONResponse(w, http.StatusOK, res)
	})

	mux.HandleFunc("/v1/browser/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SiteID     string `json:"site_id"`
			FlowAction string `json:"flow_action"`
			Profile    string `json:"profile,omitempty"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		res, err := p.service.Run(r.Context(), strings.TrimSpace(req.SiteID), strings.TrimSpace(req.FlowAction), strings.TrimSpace(req.Profile))
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSONResponse(w, http.StatusOK, res)
	})

	mux.HandleFunc("/v1/vault/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSONResponse(w, http.StatusOK, p.vaultStatus)
	})

	return []plugin.HTTPHandlerEntry{
		{Pattern: "/v1/browser/", Handler: mux},
		{Pattern: "/v1/vault/status", Handler: mux},
	}
}

func writeJSONResponse(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(data, dst); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}
