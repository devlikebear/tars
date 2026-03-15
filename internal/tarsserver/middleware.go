package tarsserver

import (
	"io"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func applyAPIMiddleware(cfg config.Config, logger zerolog.Logger, next http.Handler, authLog io.Writer) http.Handler {
	if dashboardAuthIsLoopbackOnly(cfg) {
		logger.Warn().
			Str("dashboard_auth_mode", "off").
			Str("public_access", "loopback-only").
			Msg("dashboard auth off is restricted to loopback requests")
	}
	auth := serverauth.NewMiddleware(serverauth.Options{
		Mode:                          cfg.APIAuthMode,
		BearerToken:                   cfg.APIAuthToken,
		UserToken:                     cfg.APIUserToken,
		AdminToken:                    cfg.APIAdminToken,
		RequireWorkspaceForAuthorized: false,
		SkipPaths:                     apiAuthSkipPaths(cfg),
		LoopbackSkipPaths:             dashboardLoopbackSkipPaths(cfg),
		AdminPaths:                    apiAdminPaths(),
	}, authLog)
	return requestDebugMiddleware(logger, auth(withDefaultWorkspaceBinding(next)))
}

func apiAuthSkipPaths(cfg config.Config) []string {
	return []string{"/v1/healthz"}
}

func apiAdminPaths() []string {
	return []string{
		"/v1/admin/*",
		"/v1/runtime/extensions/reload",
		"/v1/gateway/reload",
		"/v1/gateway/restart",
		"/v1/channels/webhook/inbound/*",
		"/v1/channels/telegram/webhook/*",
		"/v1/channels/telegram/pairings*",
	}
}

func withDefaultWorkspaceBinding(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next == nil {
			http.NotFound(w, r)
			return
		}
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		req := r.WithContext(serverauth.WithWorkspaceID(r.Context(), defaultWorkspaceID))
		next.ServeHTTP(w, req)
	})
}

func dashboardLoopbackSkipPaths(cfg config.Config) []string {
	if !dashboardAuthIsLoopbackOnly(cfg) {
		return nil
	}
	return []string{"/dashboards", "/dashboards/", "/ui/projects/*"}
}

func dashboardAuthHealthzStatus(cfg config.Config) map[string]any {
	if !dashboardAuthIsLoopbackOnly(cfg) {
		return nil
	}
	return map[string]any{
		"mode":          "off",
		"public_access": "loopback-only",
		"warning":       "dashboard auth off is restricted to loopback requests",
	}
}

func dashboardAuthIsLoopbackOnly(cfg config.Config) bool {
	return strings.TrimSpace(strings.ToLower(cfg.DashboardAuthMode)) == "off"
}
