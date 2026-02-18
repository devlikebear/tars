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
	WorkspaceHeader string
	SkipPaths       []string
}

type workspaceIDKey struct{}

func WorkspaceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(workspaceIDKey{}).(string)
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
	if logOut == nil {
		logOut = io.Discard
	}
	logger := log.New(logOut, "", 0)
	expectedHash := sha256.Sum256([]byte(token))

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
			if !requireToken {
				next.ServeHTTP(w, req)
				return
			}
			if token == "" {
				logger.Printf("api auth enabled but token is empty; rejecting path=%s", r.URL.Path)
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
				return
			}
			if !isValidBearerToken(r.Header.Get("Authorization"), expectedHash) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
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

func isValidBearerToken(authHeader string, expectedHash [32]byte) bool {
	if strings.TrimSpace(authHeader) == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(authHeader), strings.ToLower(prefix)) {
		return false
	}
	presentedToken := strings.TrimSpace(authHeader[len(prefix):])
	if presentedToken == "" {
		return false
	}
	presentedHash := sha256.Sum256([]byte(presentedToken))
	return subtle.ConstantTimeCompare(expectedHash[:], presentedHash[:]) == 1
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
