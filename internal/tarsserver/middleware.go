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
	auth := serverauth.NewMiddleware(serverauth.Options{
		Mode:                          cfg.APIAuthMode,
		BearerToken:                   cfg.APIAuthToken,
		UserToken:                     cfg.APIUserToken,
		AdminToken:                    cfg.APIAdminToken,
		RequireWorkspaceForAuthorized: false,
		SkipPaths:                     apiAuthSkipPaths(cfg),
		AdminPaths:                    apiAdminPaths(),
	}, authLog)
	return requestDebugMiddleware(logger, auth(withDefaultWorkspaceBinding(next)))
}

func apiAuthSkipPaths(cfg config.Config) []string {
	paths := []string{
		"/v1/healthz",
	}
	if strings.TrimSpace(strings.ToLower(cfg.DashboardAuthMode)) == "off" {
		paths = append(paths, "/dashboards", "/dashboards/", "/ui/projects/*")
	}
	return paths
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
