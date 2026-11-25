package apihandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/rs/zerolog"
)

type remoteAccessAPIHandler struct {
	cfg        config.Config
	configPath string
	logger     zerolog.Logger
	runner     remoteaccess.Runner
	targetURL  string
	relayURL   string
}

type RemoteAccessOptions struct {
	Config     config.Config
	ConfigPath string
	Logger     zerolog.Logger
	Runner     remoteaccess.Runner
	TargetURL  string
	// RelayTargetURL, when set, is the phone preview relay's listener: the
	// switch then also maps Tailscale serve's relay port (8443) to it and
	// the status reports it (docs/decisions/vibe-phone-preview.md).
	RelayTargetURL string
}

type remoteAccessPreflightCheck struct {
	Key     string `json:"key"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type remoteAccessAPIStatusResponse struct {
	DesiredEnabled   bool   `json:"desired_enabled"`
	DesiredHTTPSPort int    `json:"desired_https_port"`
	TargetURL        string `json:"target_url"`
	URL              string `json:"url,omitempty"`
	// RelayURL is the phone preview relay's address, set only while
	// Tailscale serves both TARS and the relay.
	RelayURL string                       `json:"relay_url,omitempty"`
	Status   remoteaccess.Status          `json:"status"`
	Checks   []remoteAccessPreflightCheck `json:"checks"`
}

func NewRemoteAccessHandler(opts RemoteAccessOptions) http.Handler {
	if strings.TrimSpace(opts.TargetURL) == "" {
		opts.TargetURL = remoteaccess.DefaultTargetURL
	}
	if opts.Runner == nil {
		opts.Runner = remoteaccess.ExecRunner{}
	}
	return &remoteAccessAPIHandler{
		cfg:        opts.Config,
		configPath: config.ResolveConfigPath(opts.ConfigPath),
		logger:     opts.Logger,
		runner:     opts.Runner,
		targetURL:  strings.TrimSpace(opts.TargetURL),
		relayURL:   strings.TrimSpace(opts.RelayTargetURL),
	}
}

func (h *remoteAccessAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/admin/remote-access/status":
		h.handleStatus(w, r)
	case "/v1/admin/remote-access/enable":
		h.handleEnable(w, r)
	case "/v1/admin/remote-access/disable":
		h.handleDisable(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *remoteAccessAPIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !httpapi.RequireMethod(w, r, http.MethodGet) {
		return
	}
	status, checks, err := h.detectWithChecks(r.Context(), h.currentHTTPSPort())
	if err != nil {
		httpapi.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "code": "remote_access_status_failed"})
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.statusResponse(status, checks))
}

func (h *remoteAccessAPIHandler) handleEnable(w http.ResponseWriter, r *http.Request) {
	if !httpapi.RequireMethod(w, r, http.MethodPost) {
		return
	}
	port, err := h.requestedHTTPSPort(r, h.currentHTTPSPort())
	if err != nil {
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_remote_access_request"})
		return
	}
	status, checks, err := h.detectWithChecks(r.Context(), port)
	if err != nil {
		httpapi.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "code": "remote_access_status_failed"})
		return
	}
	if failed := failedRemoteAccessChecks(checks); len(failed) > 0 {
		httpapi.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":  "remote access preflight failed",
			"code":   "remote_access_preflight_failed",
			"checks": checks,
			"status": status,
		})
		return
	}
	if err := remoteaccess.Enable(r.Context(), h.remoteOptions(port)); err != nil {
		httpapi.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "code": "remote_access_enable_failed"})
		return
	}
	// One switch, two ports. A relay port another target holds does not
	// fail the switch: the status shows the relay unserved and the phone's
	// ticket answers relay_not_served.
	if err := remoteaccess.EnableRelay(r.Context(), h.remoteOptions(port)); err != nil {
		h.logger.Warn().Err(err).Msg("preview relay: tailscale serve port not mapped")
	}
	if err := h.patchDesiredState(true, port); err != nil {
		h.logger.Error().Err(err).Str("path", h.configPath).Msg("failed to persist remote access enabled state")
		httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "code": "remote_access_persist_failed"})
		return
	}
	status, checks, err = h.detectWithChecks(r.Context(), port)
	if err != nil {
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.statusResponse(status, checks))
}

func (h *remoteAccessAPIHandler) handleDisable(w http.ResponseWriter, r *http.Request) {
	if !httpapi.RequireMethod(w, r, http.MethodPost) {
		return
	}
	port, err := h.requestedHTTPSPort(r, h.currentHTTPSPort())
	if err != nil {
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_remote_access_request"})
		return
	}
	status, err := remoteaccess.Detect(r.Context(), h.remoteOptions(port))
	if err != nil {
		httpapi.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "code": "remote_access_status_failed"})
		return
	}
	if err := remoteaccess.DisableRelay(r.Context(), h.remoteOptions(port)); err != nil {
		h.logger.Warn().Err(err).Msg("preview relay: tailscale serve port not removed")
	}
	if status.Installed && status.ServeActive && status.OwnedByTARS {
		if err := remoteaccess.Disable(r.Context(), h.remoteOptions(port)); err != nil {
			httpapi.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "code": "remote_access_disable_failed"})
			return
		}
	}
	if err := h.patchDesiredState(false, port); err != nil {
		h.logger.Error().Err(err).Str("path", h.configPath).Msg("failed to persist remote access disabled state")
		httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "code": "remote_access_persist_failed"})
		return
	}
	status, checks, err := h.detectWithChecks(r.Context(), port)
	if err != nil {
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.statusResponse(status, checks))
}

func (h *remoteAccessAPIHandler) detectWithChecks(ctx context.Context, port int) (remoteaccess.Status, []remoteAccessPreflightCheck, error) {
	status, err := remoteaccess.Detect(ctx, h.remoteOptions(port))
	if err != nil {
		return remoteaccess.Status{}, nil, err
	}
	checks := h.preflightChecks(status)
	return status, checks, nil
}

func (h *remoteAccessAPIHandler) preflightChecks(status remoteaccess.Status) []remoteAccessPreflightCheck {
	store := consoleauth.NewStore(h.cfg.WorkspaceDir)
	adminConfigured, adminErr := store.HasPassword(consoleauth.RoleAdmin)
	userConfigured, userErr := store.HasPassword(consoleauth.RoleUser)
	authModeRequired := strings.EqualFold(strings.TrimSpace(h.cfg.APIAuthMode), "required")
	return []remoteAccessPreflightCheck{
		{
			Key:     "api_auth_required",
			OK:      authModeRequired,
			Message: "api_auth_mode must be required before exposing the console through Tailscale Serve",
		},
		{
			Key:     "admin_password_configured",
			OK:      adminErr == nil && adminConfigured,
			Message: "admin browser login password must be configured",
		},
		{
			Key:     "user_password_configured",
			OK:      userErr == nil && userConfigured,
			Message: "user browser login password must be configured",
		},
		{
			Key:     "tailscale_installed",
			OK:      status.Installed,
			Message: "tailscale CLI must be installed on this Mac",
		},
		{
			Key:     "tailscale_logged_in",
			OK:      status.LoggedIn,
			Message: "tailscale must be logged in before remote access can be enabled",
		},
		{
			Key:     "serve_target_available",
			OK:      !status.ServeActive || status.OwnedByTARS,
			Message: "the configured Tailscale Serve HTTPS port must be idle or already owned by TARS",
		},
	}
}

func failedRemoteAccessChecks(checks []remoteAccessPreflightCheck) []remoteAccessPreflightCheck {
	var failed []remoteAccessPreflightCheck
	for _, check := range checks {
		if !check.OK {
			failed = append(failed, check)
		}
	}
	return failed
}

func (h *remoteAccessAPIHandler) patchDesiredState(enabled bool, port int) error {
	if strings.TrimSpace(h.configPath) == "" {
		return errors.New("config path is empty")
	}
	if port <= 0 {
		port = remoteaccess.DefaultHTTPSPort
	}
	if err := config.PatchYAML(h.configPath, map[string]any{
		"remote_access_tailscale_serve_enabled":    enabled,
		"remote_access_tailscale_serve_https_port": port,
	}); err != nil {
		return err
	}
	h.cfg.RemoteAccessTailscaleServeEnabled = enabled
	h.cfg.RemoteAccessTailscaleServeHTTPSPort = port
	return nil
}

func (h *remoteAccessAPIHandler) requestedHTTPSPort(r *http.Request, fallback int) (int, error) {
	port := fallback
	if port <= 0 {
		port = remoteaccess.DefaultHTTPSPort
	}
	if r.Body != nil && r.ContentLength != 0 {
		var req struct {
			HTTPSPort int `json:"https_port"`
			Port      int `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return 0, fmt.Errorf("invalid JSON body")
		}
		if req.HTTPSPort > 0 {
			port = req.HTTPSPort
		}
		if req.Port > 0 {
			port = req.Port
		}
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("https port must be between 1 and 65535")
	}
	return port, nil
}

func (h *remoteAccessAPIHandler) currentHTTPSPort() int {
	if h.cfg.RemoteAccessTailscaleServeHTTPSPort > 0 {
		return h.cfg.RemoteAccessTailscaleServeHTTPSPort
	}
	return remoteaccess.DefaultHTTPSPort
}

func (h *remoteAccessAPIHandler) remoteOptions(port int) remoteaccess.Options {
	return remoteaccess.Options{
		Runner:         h.runner,
		HTTPSPort:      port,
		TargetURL:      h.targetURL,
		RelayTargetURL: h.relayURL,
	}
}

func (h *remoteAccessAPIHandler) statusResponse(status remoteaccess.Status, checks []remoteAccessPreflightCheck) remoteAccessAPIStatusResponse {
	return remoteAccessAPIStatusResponse{
		DesiredEnabled:   h.cfg.RemoteAccessTailscaleServeEnabled,
		DesiredHTTPSPort: h.currentHTTPSPort(),
		TargetURL:        h.targetURL,
		URL:              remoteAccessURL(status, h.currentHTTPSPort()),
		RelayURL:         status.RelayURL(),
		Status:           status,
		Checks:           checks,
	}
}

func remoteAccessURL(status remoteaccess.Status, port int) string {
	if strings.TrimSpace(status.TailnetURL) == "" {
		return ""
	}
	url := "https://" + strings.TrimSpace(status.TailnetURL)
	if port > 0 && port != 443 {
		url = fmt.Sprintf("%s:%d", url, port)
	}
	return url
}
