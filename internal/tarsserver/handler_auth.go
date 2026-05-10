package tarsserver

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/serverauth"
)

const browserSessionTTL = 30 * 24 * time.Hour

type LoginGate struct {
	Allowed      bool
	CookieSecure bool
	Reason       string
}

type authAPIHandler struct {
	mode         string
	store        *consoleauth.Store
	failures     *consoleauth.FailureLimiter
	sessionTTL   time.Duration
	cookieName   string
	cookieMaxAge int
}

func newAuthAPIHandler(authMode, workspaceDir string) http.Handler {
	mode := serverauth.NormalizeMode(authMode)
	return &authAPIHandler{
		mode:         mode,
		store:        consoleauth.NewStore(workspaceDir),
		failures:     consoleauth.NewFailureLimiter(5, time.Minute),
		sessionTTL:   browserSessionTTL,
		cookieName:   serverauth.DefaultBrowserSessionCookieName,
		cookieMaxAge: int(browserSessionTTL.Seconds()),
	}
}

func (h *authAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/auth/whoami":
		h.handleWhoami(w, r)
	case "/v1/auth/login":
		h.handleLogin(w, r)
	case "/v1/auth/pairing-login":
		h.handlePairingLogin(w, r)
	case "/v1/auth/logout":
		h.handleLogout(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/v1/auth/users/") {
			h.handleUserPassword(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

func (h *authAPIHandler) handleWhoami(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	role := strings.TrimSpace(serverauth.RoleFromRequest(r))
	adminConfigured, _ := h.store.HasPassword(consoleauth.RoleAdmin)
	userConfigured, _ := h.store.HasPassword(consoleauth.RoleUser)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":    role != "",
		"auth_role":        role,
		"is_admin":         role == serverauth.RoleAdmin,
		"auth_mode":        h.mode,
		"admin_configured": adminConfigured,
		"user_configured":  userConfigured,
	})
}

func (h *authAPIHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	gate := evaluateLoginRequest(r)
	if !gate.Allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": gate.Reason,
			"code":  "login_not_allowed",
		})
		return
	}
	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login request", "code": "invalid_request"})
		return
	}
	role, err := normalizeLoginRole(firstNonEmpty(req.Username, req.Role))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_role"})
		return
	}
	if role == consoleauth.RoleAdmin && loginRequestIsRemote(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "remote admin login is not allowed", "code": "remote_admin_forbidden"})
		return
	}
	remoteKey := loginFailureRemoteKey(r)
	if allowed, until := h.failures.Allow(consoleauth.FailurePurposeLogin, role, remoteKey); !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "too many login attempts",
			"code":  "login_locked",
			"until": until.Format(time.RFC3339),
		})
		return
	}
	ok, err := h.store.VerifyPassword(role, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials", "code": "invalid_credentials"})
		return
	}
	if !ok {
		h.failures.RecordFailure(consoleauth.FailurePurposeLogin, role, remoteKey)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials", "code": "invalid_credentials"})
		return
	}
	h.failures.RecordSuccess(consoleauth.FailurePurposeLogin, role, remoteKey)
	session, err := h.store.CreateSession(role, userAgentHint(r), h.sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session", "code": "session_create_failed"})
		return
	}
	h.writeSessionCookie(w, session.ID, gate.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"auth_role":     role,
		"is_admin":      role == consoleauth.RoleAdmin,
		"auth_mode":     h.mode,
	})
}

func (h *authAPIHandler) handlePairingLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	gate := evaluateLoginRequest(r)
	if !gate.Allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": gate.Reason,
			"code":  "login_not_allowed",
		})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing request", "code": "invalid_request"})
		return
	}
	remoteKey := loginFailureRemoteKey(r)
	if allowed, until := h.failures.Allow(consoleauth.FailurePurposePairing, consoleauth.RoleUser, remoteKey); !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "too many pairing attempts",
			"code":  "pairing_locked",
			"until": until.Format(time.RFC3339),
		})
		return
	}
	pairing, ok, err := h.store.ConsumePairingCode(req.Code)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to consume pairing code", "code": "pairing_failed"})
		return
	}
	if !ok {
		h.failures.RecordFailure(consoleauth.FailurePurposePairing, consoleauth.RoleUser, remoteKey)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid pairing code", "code": "invalid_pairing_code"})
		return
	}
	if pairing.Role != consoleauth.RoleUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "pairing code role is not supported for browser login", "code": "unsupported_pairing_role"})
		return
	}
	h.failures.RecordSuccess(consoleauth.FailurePurposePairing, consoleauth.RoleUser, remoteKey)
	session, err := h.store.CreateSession(consoleauth.RoleUser, userAgentHint(r), h.sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session", "code": "session_create_failed"})
		return
	}
	h.writeSessionCookie(w, session.ID, gate.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"auth_role":     consoleauth.RoleUser,
		"is_admin":      false,
		"auth_mode":     h.mode,
	})
}

func (h *authAPIHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if cookie, err := r.Cookie(h.cookieName); err == nil && cookie != nil {
		if err := h.store.RevokeSession(cookie.Value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke session", "code": "logout_failed"})
			return
		}
	}
	// Secure must match the request class so loopback HTTP can clear local
	// development cookies while HTTPS/Tailscale flows clear Secure cookies.
	h.writeClearSessionCookie(w, shouldClearSecureCookie(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *authAPIHandler) handleUserPassword(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPatch) {
		return
	}
	target := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/auth/users/"), "/")
	if strings.HasSuffix(target, "/password") {
		target = strings.TrimSuffix(target, "/password")
	}
	targetRole, err := normalizeLoginRole(target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found", "code": "user_not_found"})
		return
	}
	requestRole := strings.TrimSpace(serverauth.RoleFromRequest(r))
	if requestRole != serverauth.RoleAdmin && requestRole != targetRole {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "code": "forbidden"})
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		Password        string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid password request", "code": "invalid_request"})
		return
	}
	newPassword := firstNonEmpty(req.NewPassword, req.Password)
	if requestRole != serverauth.RoleAdmin {
		ok, err := h.store.VerifyPassword(targetRole, req.CurrentPassword)
		if err != nil || !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid current password", "code": "invalid_current_password"})
			return
		}
	}
	if err := h.store.SetPassword(targetRole, newPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_password"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"role": targetRole,
	})
}

// writeSessionCookie emits the browser session cookie, branching on the request
// class decided by evaluateLoginRequest so each call site of http.SetCookie has a
// literal Secure value that CodeQL/Sonar can reason about. The non-Secure branch
// is reachable only when evaluateLoginRequest classified the request as local
// loopback (127.0.0.1 / localhost / ::1); remote HTTPS and Tailscale-authenticated
// flows always take the Secure: true branch.
func (h *authAPIHandler) writeSessionCookie(w http.ResponseWriter, sessionID string, secure bool) {
	if secure {
		http.SetCookie(w, &http.Cookie{
			Name:     h.cookieName,
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			MaxAge:   h.cookieMaxAge,
		})
		return
	}
	// Loopback-only fallback. evaluateLoginRequest gates this branch so a
	// non-Secure cookie cannot escape 127.0.0.1/localhost/::1; insecure non-local
	// HTTP is rejected with 403 before we ever reach this code path.
	http.SetCookie(w, &http.Cookie{ // NOSONAR -- gated to loopback by evaluateLoginRequest
		Name:     h.cookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   h.cookieMaxAge,
	})
}

// writeClearSessionCookie mirrors writeSessionCookie for the logout path. The
// Secure attribute on a clearing cookie must match the original cookie or the
// browser silently keeps the stale cookie around, so we still need the loopback
// branch — but each branch carries an explicit Secure literal.
func (h *authAPIHandler) writeClearSessionCookie(w http.ResponseWriter, secure bool) {
	if secure {
		http.SetCookie(w, &http.Cookie{
			Name:     h.cookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			MaxAge:   -1,
		})
		return
	}

	// codeql[go/cookie-secure-not-set] -- This only clears the loopback-only HTTP cookie emitted above; Secure=false must match that cookie so browsers actually delete it.
	http.SetCookie(w, &http.Cookie{ // NOSONAR -- clears the loopback-only cookie set by writeSessionCookie
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   -1,
	})
}

func evaluateLoginRequest(r *http.Request) LoginGate {
	if r == nil {
		return LoginGate{Allowed: false, Reason: "login request is required"}
	}
	if isLocalLoginHost(r.Host) {
		return LoginGate{Allowed: true, CookieSecure: false}
	}
	if serverauth.HasTailscaleIdentityHeader(r) && (isLoopbackLoginRemoteAddr(r.RemoteAddr) || r.TLS != nil) {
		return LoginGate{Allowed: true, CookieSecure: true}
	}
	if isTailscaleDNSHost(r.Host) {
		return LoginGate{Allowed: false, Reason: "Tailscale tagged device is not supported for browser login"}
	}
	if r.TLS != nil {
		return LoginGate{Allowed: true, CookieSecure: true}
	}
	return LoginGate{Allowed: false, Reason: "secure connection required"}
}

func shouldClearSecureCookie(r *http.Request) bool {
	gate := evaluateLoginRequest(r)
	return gate.CookieSecure
}

func normalizeLoginRole(role string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case consoleauth.RoleAdmin:
		return consoleauth.RoleAdmin, nil
	case consoleauth.RoleUser:
		return consoleauth.RoleUser, nil
	default:
		return "", fmt.Errorf("username must be admin or user")
	}
}

func loginRequestIsRemote(r *http.Request) bool {
	if r == nil {
		return false
	}
	return !isLocalLoginHost(r.Host) || serverauth.HasTailscaleIdentityHeader(r)
}

func isLocalLoginHost(host string) bool {
	host = normalizeLoginHost(host)
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func isTailscaleDNSHost(host string) bool {
	host = normalizeLoginHost(host)
	return strings.HasSuffix(host, ".ts.net")
}

func normalizeLoginHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = strings.Trim(parsed, "[]")
	}
	host = strings.Trim(host, "[]")
	return host
}

func isLoopbackLoginRemoteAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	host = strings.Trim(host, "[]")
	if idx := strings.LastIndex(host, "%"); idx > 0 {
		host = host[:idx]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loginFailureRemoteKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if login := strings.TrimSpace(r.Header.Get(serverauth.TailscaleUserLoginHeader)); login != "" {
		return "tailscale:" + strings.ToLower(login)
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func userAgentHint(r *http.Request) string {
	if r == nil {
		return ""
	}
	value := strings.TrimSpace(r.UserAgent())
	if len(value) > 160 {
		return value[:160]
	}
	return value
}
