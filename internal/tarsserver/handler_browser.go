package tarsserver

import (
	"encoding/json"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/rs/zerolog"
)

type browserRelayInfoProvider interface {
	Addr() string
	RelayToken() string
	ExtensionConnected() bool
	AttachedTabs() int
	CDPWebSocketURL() string
	AuthRequired() bool
	JSONAuthRequired() bool
}

func newBrowserAPIHandler(
	runtime *gateway.Runtime,
	vaultStatus vaultStatusSnapshot,
	relay browserRelayInfoProvider,
	relayEnabled bool,
	relayOriginAllowlist []string,
	logger zerolog.Logger,
) http.Handler {
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
	mux.HandleFunc("/v1/browser/relay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		addr := ""
		token := ""
		extensionConnected := false
		attachedTabs := 0
		cdpWSURL := ""
		authRequired := false
		jsonAuthRequired := false
		if relay != nil {
			addr = strings.TrimSpace(relay.Addr())
			token = strings.TrimSpace(relay.RelayToken())
			extensionConnected = relay.ExtensionConnected()
			attachedTabs = relay.AttachedTabs()
			cdpWSURL = strings.TrimSpace(relay.CDPWebSocketURL())
			authRequired = relay.AuthRequired()
			jsonAuthRequired = relay.JSONAuthRequired()
		}
		isAdmin := strings.TrimSpace(serverauth.RoleFromRequest(r)) == serverauth.RoleAdmin
		extensionWSURL := ""
		if addr != "" {
			extensionWSURL = "ws://" + addr + "/extension"
		}
		if !isAdmin {
			cdpWSURL = redactRelayCDPWebSocketURL(cdpWSURL, addr)
		}
		payload := map[string]any{
			"enabled":             relayEnabled,
			"running":             strings.TrimSpace(addr) != "",
			"addr":                addr,
			"extension_connected": extensionConnected,
			"attached_tabs":       attachedTabs,
			"extension_ws_url":    extensionWSURL,
			"cdp_ws_url":          cdpWSURL,
			"origin_allowlist":    append([]string(nil), relayOriginAllowlist...),
			"auth_required":       authRequired,
			"json_auth_required":  jsonAuthRequired,
		}
		if isAdmin && token != "" {
			payload["relay_token"] = token
		}
		writeJSON(w, http.StatusOK, payload)
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

func redactRelayCDPWebSocketURL(cdpWSURL, addr string) string {
	trimmed := strings.TrimSpace(cdpWSURL)
	if trimmed == "" {
		if strings.TrimSpace(addr) == "" {
			return ""
		}
		return "ws://" + strings.TrimSpace(addr) + "/cdp"
	}
	parsed, err := neturl.Parse(trimmed)
	if err != nil {
		if idx := strings.Index(trimmed, "?"); idx >= 0 {
			return trimmed[:idx]
		}
		return trimmed
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	return parsed.String()
}
