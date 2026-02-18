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
		Mode:            cfg.APIAuthMode,
		BearerToken:     cfg.APIAuthToken,
		WorkspaceHeader: cfg.APIWorkspaceHeader,
		SkipPaths:       []string{"/v1/healthz"},
	}, authLog)
	return requestDebugMiddleware(logger, auth(next))
}
