package tarsdapp

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/serverauth"
)

func newAuthAPIHandler(authMode string) http.Handler {
	mode := serverauth.NormalizeMode(authMode)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
