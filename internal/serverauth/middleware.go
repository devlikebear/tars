package serverauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
)

const (
	ModeOff              = "off"
	ModeExternalRequired = "external-required"
	ModeRequired         = "required"

	DefaultWorkspaceHeader = "Tars-Workspace-Id"
	debugWorkspaceHeader   = "Tars-Debug-Workspace-Id"
	debugRoleHeader        = "Tars-Debug-Auth-Role"
)

type Options struct {
	Mode                          string
	BearerToken                   string
	UserToken                     string
	AdminToken                    string
	WorkspaceHeader               string
	RequireWorkspaceForAuthorized bool
	UserWorkspaceAllowlist        []string
	AdminWorkspaceAllowlist       []string
	SkipPaths                     []string
	LoopbackSkipPaths             []string
	AdminPaths                    []string
}

type pathMatcher struct {
	exact    map[string]struct{}
	prefixes []string
}

type compiledOptions struct {
	mode                          string
	workspaceHeader               string
	skipPaths                     pathMatcher
	loopbackSkipPaths             pathMatcher
	adminPaths                    pathMatcher
	requireWorkspaceForAuthorized bool
	userWorkspaceAllowlist        map[string]struct{}
	adminWorkspaceAllowlist       map[string]struct{}
	logger                        zerolog.Logger
	hasLegacyToken                bool
	hasUserToken                  bool
	hasAdminToken                 bool
	anyTokenConfigured            bool
	legacyHash                    [32]byte
	userHash                      [32]byte
	adminHash                     [32]byte
}

type requestRequirement struct {
	skip         bool
	requireToken bool
	isAdminPath  bool
	tokenNeeded  bool
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

func WithWorkspaceID(ctx context.Context, workspaceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(workspaceID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, workspaceIDKey{}, trimmed)
}

func WorkspaceIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := WorkspaceIDFromContext(r.Context()); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get(debugWorkspaceHeader))
}

func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(roleKey{}).(string)
	return strings.TrimSpace(value)
}

func RoleFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := RoleFromContext(r.Context()); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get(debugRoleHeader))
}

func NormalizeMode(raw string) string {
	mode := strings.TrimSpace(strings.ToLower(raw))
	switch mode {
	case ModeOff, ModeExternalRequired, ModeRequired:
		return mode
	default:
		return ModeRequired
	}
}

func compileOptions(opts Options, logOut io.Writer) compiledOptions {
	workspaceHeader := strings.TrimSpace(opts.WorkspaceHeader)
	if workspaceHeader == "" {
		workspaceHeader = DefaultWorkspaceHeader
	}
	if logOut == nil {
		logOut = io.Discard
	}

	token := strings.TrimSpace(opts.BearerToken)
	userToken := strings.TrimSpace(opts.UserToken)
	adminToken := strings.TrimSpace(opts.AdminToken)
	hasLegacyToken := token != ""
	hasUserToken := userToken != ""
	hasAdminToken := adminToken != ""

	return compiledOptions{
		mode:                          NormalizeMode(opts.Mode),
		workspaceHeader:               workspaceHeader,
		skipPaths:                     newPathMatcher(opts.SkipPaths),
		loopbackSkipPaths:             newPathMatcher(opts.LoopbackSkipPaths),
		adminPaths:                    newPathMatcher(opts.AdminPaths),
		requireWorkspaceForAuthorized: opts.RequireWorkspaceForAuthorized,
		userWorkspaceAllowlist:        toWorkspaceAllowlist(opts.UserWorkspaceAllowlist),
		adminWorkspaceAllowlist:       toWorkspaceAllowlist(opts.AdminWorkspaceAllowlist),
		logger:                        zerolog.New(logOut).With().Str("component", "serverauth").Logger(),
		hasLegacyToken:                hasLegacyToken,
		hasUserToken:                  hasUserToken,
		hasAdminToken:                 hasAdminToken,
		anyTokenConfigured:            hasLegacyToken || hasUserToken || hasAdminToken,
		legacyHash:                    sha256.Sum256([]byte(token)),
		userHash:                      sha256.Sum256([]byte(userToken)),
		adminHash:                     sha256.Sum256([]byte(adminToken)),
	}
}

func NewMiddleware(opts Options, logOut io.Writer) func(http.Handler) http.Handler {
	compiled := compileOptions(opts, logOut)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := withWorkspaceID(r, compiled.workspaceHeader)
			req = withDebugWorkspaceHeader(req)
			requirement := compiled.requirementForRequest(r)
			if compiled.mode == ModeOff || requirement.skip {
				next.ServeHTTP(w, req)
				return
			}

			if requirement.tokenNeeded && !compiled.anyTokenConfigured {
				compiled.logger.Warn().Str("path", r.URL.Path).Msg("api auth enabled but token is empty; rejecting request")
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}

			_, hasBearer := parseBearerToken(r.Header.Get("Authorization"))
			role := compiled.resolveRole(r.Header.Get("Authorization"))
			if requirement.tokenNeeded && role == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			req = withRole(req, role)
			req = withDebugRoleHeader(req, role)
			if compiled.requireWorkspaceForAuthorized && strings.TrimSpace(role) != "" {
				if strings.TrimSpace(WorkspaceIDFromContext(req.Context())) == "" {
					writeJSONError(w, http.StatusBadRequest, "workspace_id_required", "workspace id is required")
					return
				}
			}
			if role == RoleUser && !isWorkspaceAllowed(compiled.userWorkspaceAllowlist, WorkspaceIDFromContext(req.Context())) {
				writeJSONError(w, http.StatusForbidden, "forbidden", "forbidden")
				return
			}
			if role == RoleAdmin && !isWorkspaceAllowed(compiled.adminWorkspaceAllowlist, WorkspaceIDFromContext(req.Context())) {
				writeJSONError(w, http.StatusForbidden, "forbidden", "forbidden")
				return
			}
			if requirement.isAdminPath && role != RoleAdmin {
				writeJSONError(w, http.StatusForbidden, "forbidden", "forbidden")
				return
			}
			if requirement.requireToken && hasBearer && role == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func newPathMatcher(paths []string) pathMatcher {
	matcher := pathMatcher{
		exact:    make(map[string]struct{}, len(paths)),
		prefixes: make([]string, 0, len(paths)),
	}
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, "*") {
			prefix := strings.TrimSpace(strings.TrimSuffix(trimmed, "*"))
			if prefix != "" {
				matcher.prefixes = append(matcher.prefixes, prefix)
			}
			continue
		}
		matcher.exact[trimmed] = struct{}{}
	}
	return matcher
}

func (m pathMatcher) match(path string) bool {
	if _, ok := m.exact[path]; ok {
		return true
	}
	for _, prefix := range m.prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (c compiledOptions) requirementForRequest(r *http.Request) requestRequirement {
	if r == nil {
		return requestRequirement{}
	}
	if c.skipPaths.match(r.URL.Path) {
		return requestRequirement{skip: true}
	}
	if c.loopbackSkipPaths.match(r.URL.Path) && isLoopbackRemoteAddr(r.RemoteAddr) {
		return requestRequirement{skip: true}
	}
	requireToken := c.mode == ModeRequired
	if c.mode == ModeExternalRequired && !isLoopbackRemoteAddr(r.RemoteAddr) {
		requireToken = true
	}
	isAdminPath := c.adminPaths.match(r.URL.Path)
	return requestRequirement{
		requireToken: requireToken,
		isAdminPath:  isAdminPath,
		tokenNeeded:  requireToken || isAdminPath,
	}
}

func (c compiledOptions) resolveRole(authHeader string) string {
	presentedToken, hasBearer := parseBearerToken(authHeader)
	if !hasBearer {
		return ""
	}
	return resolveTokenRole(
		presentedToken,
		c.hasLegacyToken,
		c.hasUserToken,
		c.hasAdminToken,
		c.legacyHash,
		c.userHash,
		c.adminHash,
	)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	if strings.TrimSpace(code) == "" {
		code = strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	}
	if strings.TrimSpace(message) == "" {
		message = code
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
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

func withDebugWorkspaceHeader(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	workspaceID := WorkspaceIDFromContext(r.Context())
	if workspaceID == "" {
		r.Header.Del(debugWorkspaceHeader)
		return r
	}
	r.Header.Set(debugWorkspaceHeader, workspaceID)
	return r
}

func withDebugRoleHeader(r *http.Request, role string) *http.Request {
	if r == nil {
		return nil
	}
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		r.Header.Del(debugRoleHeader)
		return r
	}
	r.Header.Set(debugRoleHeader, trimmed)
	return r
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

func toWorkspaceAllowlist(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func isWorkspaceAllowed(allowlist map[string]struct{}, workspaceID string) bool {
	if len(allowlist) == 0 {
		return true
	}
	_, ok := allowlist[strings.TrimSpace(workspaceID)]
	return ok
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
