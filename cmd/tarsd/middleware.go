package main

import (
	"io"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/rs/zerolog"
)

func applyAPIMiddleware(cfg config.Config, logger zerolog.Logger, next http.Handler, authLog io.Writer) http.Handler {
	binding := buildRoleWorkspaceBinding(cfg)
	auth := serverauth.NewMiddleware(serverauth.Options{
		Mode:                          cfg.APIAuthMode,
		BearerToken:                   cfg.APIAuthToken,
		UserToken:                     cfg.APIUserToken,
		AdminToken:                    cfg.APIAdminToken,
		WorkspaceHeader:               cfg.APIWorkspaceHeader,
		RequireWorkspaceForAuthorized: false,
		SkipPaths:                     apiAuthSkipPaths(),
		AdminPaths:                    apiAdminPaths(),
	}, authLog)
	return requestDebugMiddleware(logger, auth(withRoleWorkspaceBinding(next, binding)))
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

type roleWorkspaceBinding struct {
	header string
	user   string
	admin  string
}

func buildRoleWorkspaceBinding(cfg config.Config) roleWorkspaceBinding {
	header := strings.TrimSpace(cfg.APIWorkspaceHeader)
	if header == "" {
		header = serverauth.DefaultWorkspaceHeader
	}
	return roleWorkspaceBinding{
		header: header,
		user:   firstWorkspace(cfg.APIUserWorkspaceIDs),
		admin:  firstWorkspace(cfg.APIAdminWorkspaceIDs),
	}
}

func firstWorkspace(values []string) string {
	for _, value := range values {
		trimmed := normalizeWorkspaceID(value)
		if trimmed != defaultWorkspaceID || strings.TrimSpace(value) != "" {
			return trimmed
		}
	}
	return ""
}

func withRoleWorkspaceBinding(next http.Handler, binding roleWorkspaceBinding) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next == nil {
			http.NotFound(w, r)
			return
		}
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		workspaceID := resolveWorkspaceForRequest(r, binding)
		if strings.TrimSpace(workspaceID) == "" {
			workspaceID = defaultWorkspaceID
		}
		workspaceID = normalizeWorkspaceID(workspaceID)
		req := r.WithContext(serverauth.WithWorkspaceID(r.Context(), workspaceID))
		req.Header.Set("Tars-Debug-Workspace-Id", workspaceID)
		next.ServeHTTP(w, req)
	})
}

func resolveWorkspaceForRequest(r *http.Request, binding roleWorkspaceBinding) string {
	role := strings.TrimSpace(serverauth.RoleFromContext(r.Context()))
	switch role {
	case serverauth.RoleUser:
		if strings.TrimSpace(binding.user) != "" {
			return binding.user
		}
	case serverauth.RoleAdmin:
		if strings.TrimSpace(binding.admin) != "" {
			return binding.admin
		}
	}
	workspaceID := strings.TrimSpace(serverauth.WorkspaceIDFromContext(r.Context()))
	if workspaceID != "" {
		return workspaceID
	}
	return strings.TrimSpace(r.Header.Get(binding.header))
}
