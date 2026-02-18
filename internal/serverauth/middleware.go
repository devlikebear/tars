package serverauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	ModeOff              = "off"
	ModeExternalRequired = "external-required"
	ModeRequired         = "required"

	DefaultWorkspaceHeader = "Tars-Workspace-Id"
)

type Options struct {
	Mode            string
	BearerToken     string
	UserToken       string
	AdminToken      string
	WorkspaceHeader string
	SkipPaths       []string
	AdminPaths      []string
}

type workspaceIDKey struct{}
type roleKey struct{}

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

func WorkspaceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(workspaceIDKey{}).(string)
	return strings.TrimSpace(value)
}

func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(roleKey{}).(string)
	return strings.TrimSpace(value)
}

func NormalizeMode(raw string) string {
	mode := strings.TrimSpace(strings.ToLower(raw))
	switch mode {
	case ModeOff, ModeExternalRequired, ModeRequired:
		return mode
	default:
		return ModeExternalRequired
	}
}

func NewMiddleware(opts Options, logOut io.Writer) func(http.Handler) http.Handler {
	mode := NormalizeMode(opts.Mode)
	token := strings.TrimSpace(opts.BearerToken)
	workspaceHeader := strings.TrimSpace(opts.WorkspaceHeader)
	if workspaceHeader == "" {
		workspaceHeader = DefaultWorkspaceHeader
	}
	skipPaths := make(map[string]struct{}, len(opts.SkipPaths))
	for _, path := range opts.SkipPaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		skipPaths[trimmed] = struct{}{}
	}
	adminPaths := make(map[string]struct{}, len(opts.AdminPaths))
	for _, path := range opts.AdminPaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		adminPaths[trimmed] = struct{}{}
	}
	if logOut == nil {
		logOut = io.Discard
	}
	logger := log.New(logOut, "", 0)

	userToken := strings.TrimSpace(opts.UserToken)
	adminToken := strings.TrimSpace(opts.AdminToken)
	hasLegacyToken := token != ""
	hasUserToken := userToken != ""
	hasAdminToken := adminToken != ""
	anyTokenConfigured := hasLegacyToken || hasUserToken || hasAdminToken
	legacyHash := sha256.Sum256([]byte(token))
	userHash := sha256.Sum256([]byte(userToken))
	adminHash := sha256.Sum256([]byte(adminToken))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := withWorkspaceID(r, workspaceHeader)
			if _, ok := skipPaths[r.URL.Path]; ok || mode == ModeOff {
				next.ServeHTTP(w, req)
				return
			}

			requireToken := mode == ModeRequired
			if mode == ModeExternalRequired && !isLoopbackRemoteAddr(r.RemoteAddr) {
				requireToken = true
			}
			_, isAdminPath := adminPaths[r.URL.Path]
			tokenNeeded := requireToken || isAdminPath
			if tokenNeeded && !anyTokenConfigured {
				logger.Printf("api auth enabled but token is empty; rejecting path=%s", r.URL.Path)
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
				return
			}

			presentedToken, hasBearer := parseBearerToken(r.Header.Get("Authorization"))
			role := ""
			if hasBearer {
				role = resolveTokenRole(
					presentedToken,
					hasLegacyToken,
					hasUserToken,
					hasAdminToken,
					legacyHash,
					userHash,
					adminHash,
				)
			}
			if tokenNeeded && role == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if isAdminPath && role != RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if requireToken && hasBearer && role == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, withRole(req, role))
		})
	}
}

func withWorkspaceID(r *http.Request, headerName string) *http.Request {
	if r == nil {
		return nil
	}
	workspaceID := strings.TrimSpace(r.Header.Get(headerName))
	if workspaceID == "" {
		return r
	}
	ctx := context.WithValue(r.Context(), workspaceIDKey{}, workspaceID)
	return r.WithContext(ctx)
}

func withRole(r *http.Request, role string) *http.Request {
	if r == nil {
		return nil
	}
	if strings.TrimSpace(role) == "" {
		return r
	}
	ctx := context.WithValue(r.Context(), roleKey{}, strings.TrimSpace(role))
	return r.WithContext(ctx)
}

func parseBearerToken(authHeader string) (string, bool) {
	if strings.TrimSpace(authHeader) == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(authHeader), strings.ToLower(prefix)) {
		return "", false
	}
	presentedToken := strings.TrimSpace(authHeader[len(prefix):])
	if presentedToken == "" {
		return "", false
	}
	return presentedToken, true
}

func resolveTokenRole(
	presentedToken string,
	hasLegacyToken bool,
	hasUserToken bool,
	hasAdminToken bool,
	legacyHash [32]byte,
	userHash [32]byte,
	adminHash [32]byte,
) string {
	if strings.TrimSpace(presentedToken) == "" {
		return ""
	}
	presentedHash := sha256.Sum256([]byte(presentedToken))
	if hasAdminToken && subtle.ConstantTimeCompare(adminHash[:], presentedHash[:]) == 1 {
		return RoleAdmin
	}
	if hasUserToken && subtle.ConstantTimeCompare(userHash[:], presentedHash[:]) == 1 {
		return RoleUser
	}
	if hasLegacyToken && subtle.ConstantTimeCompare(legacyHash[:], presentedHash[:]) == 1 {
		return RoleAdmin
	}
	return ""
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	value := strings.TrimSpace(remoteAddr)
	if value == "" {
		return false
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	if idx := strings.LastIndex(host, "%"); idx > 0 {
		host = host[:idx]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
