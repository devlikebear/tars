package main

import (
	"io"
	"net/http"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/rs/zerolog"
)

func applyAPIMiddleware(cfg config.Config, logger zerolog.Logger, next http.Handler, authLog io.Writer) http.Handler {
	auth := serverauth.NewMiddleware(serverauth.Options{
		Mode:                          cfg.APIAuthMode,
		BearerToken:                   cfg.APIAuthToken,
		UserToken:                     cfg.APIUserToken,
		AdminToken:                    cfg.APIAdminToken,
		WorkspaceHeader:               cfg.APIWorkspaceHeader,
		UserWorkspaceAllowlist:        cfg.APIUserWorkspaceIDs,
		AdminWorkspaceAllowlist:       cfg.APIAdminWorkspaceIDs,
		RequireWorkspaceForAuthorized: true,
		SkipPaths:                     apiAuthSkipPaths(),
		AdminPaths:                    apiAdminPaths(),
	}, authLog)
	return requestDebugMiddleware(logger, auth(next))
}

func apiAuthSkipPaths() []string {
	return []string{
		"/v1/healthz",
	}
}

func apiAdminPaths() []string {
	return []string{
		"/v1/runtime/extensions/reload",
		"/v1/gateway/reload",
		"/v1/gateway/restart",
		"/v1/channels/webhook/inbound/*",
		"/v1/channels/telegram/webhook/*",
	}
}
