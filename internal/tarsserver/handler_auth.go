package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/serverauth"
)

func newAuthAPIHandler(authMode string) http.Handler {
	mode := serverauth.NormalizeMode(authMode)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		role := strings.TrimSpace(serverauth.RoleFromRequest(r))
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": role != "",
			"auth_role":     role,
			"is_admin":      role == serverauth.RoleAdmin,
			"auth_mode":     mode,
		})
	})
}
