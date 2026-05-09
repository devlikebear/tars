package tarsserver

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func TestAuthWhoamiAPI_ReturnsRole(t *testing.T) {
	cfg := config.Config{
		APIConfig: config.APIConfig{
			APIAuthMode:  "required",
			APIUserToken: "user-token",
		},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), newAuthAPIHandler(cfg.APIAuthMode, cfg.WorkspaceDir), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/whoami", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool   `json:"authenticated"`
		AuthRole      string `json:"auth_role"`
		AuthMode      string `json:"auth_mode"`
		IsAdmin       bool   `json:"is_admin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode whoami response: %v", err)
	}
	if !body.Authenticated {
		t.Fatalf("expected authenticated=true, got %+v", body)
	}
	if body.AuthRole != "user" {
		t.Fatalf("expected auth_role user, got %+v", body)
	}
	if body.AuthMode != "required" {
		t.Fatalf("expected auth_mode required, got %+v", body)
	}
	if body.IsAdmin {
		t.Fatalf("expected is_admin false, got %+v", body)
	}
}

func TestAuthWhoamiAPI_OffModeGrantsAdminRole(t *testing.T) {
	cfg := config.Config{
		APIConfig: config.APIConfig{
			APIAuthMode: "off",
		},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), newAuthAPIHandler(cfg.APIAuthMode, cfg.WorkspaceDir), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/whoami", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool   `json:"authenticated"`
		AuthRole      string `json:"auth_role"`
		AuthMode      string `json:"auth_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode whoami response: %v", err)
	}
	if !body.Authenticated {
		t.Fatalf("expected authenticated=true in off mode, got %+v", body)
	}
	if body.AuthRole != "admin" {
		t.Fatalf("expected admin role in off mode, got %+v", body)
	}
	if body.AuthMode != "off" {
		t.Fatalf("expected auth_mode off, got %+v", body)
	}
}

func TestAuthWhoamiAPI_RejectsNonGet(t *testing.T) {
	handler := newAuthAPIHandler("required", t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/whoami", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "method not allowed\n" {
		t.Fatalf("expected plain text method-not-allowed body, got %q", rec.Body.String())
	}
}

func TestAuthLoginAPI_LocalSetsInsecureCookie(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	if err := store.SetPassword(consoleauth.RoleAdmin, "admin secret"); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"admin","password":"admin secret"}`))
	req.Host = "127.0.0.1:43180"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName)
	if cookie == nil {
		t.Fatalf("expected session cookie")
	}
	if cookie.HttpOnly != true || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("unexpected cookie attributes: %+v", cookie)
	}
	if cookie.Secure {
		t.Fatalf("loopback login cookie must not be Secure")
	}
	if _, ok, err := store.ValidateSession(cookie.Value); err != nil || !ok {
		t.Fatalf("expected created session to validate, ok=%v err=%v", ok, err)
	}
}

func TestAuthLoginAPI_TailscaleSetsSecureCookieAndRejectsAdmin(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	if err := store.SetPassword(consoleauth.RoleAdmin, "admin secret"); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	if err := store.SetPassword(consoleauth.RoleUser, "user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"user","password":"user secret"}`))
	req.Host = "macbook.tailnet.ts.net"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(serverauth.TailscaleUserLoginHeader, "user@example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName)
	if cookie == nil || !cookie.Secure {
		t.Fatalf("expected secure Tailscale session cookie, got %+v", cookie)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"admin","password":"admin secret"}`))
	req.Host = "macbook.tailnet.ts.net"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(serverauth.TailscaleUserLoginHeader, "user@example.com")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected remote admin login to be forbidden, got %d body=%q", rec.Code, rec.Body.String())
	}
	if findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName) != nil {
		t.Fatalf("remote admin rejection must not set a session cookie")
	}
}

func TestAuthPairingLoginAPI_ConsumesUserCodeAndSetsCookie(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	code, err := store.CreatePairingCode(consoleauth.RoleUser, time.Minute)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/pairing-login", strings.NewReader(`{"code":"`+code.Code+`"}`))
	req.Host = "macbook.tailnet.ts.net"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(serverauth.TailscaleUserLoginHeader, "user@example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName)
	if cookie == nil || !cookie.Secure {
		t.Fatalf("expected secure pairing session cookie, got %+v", cookie)
	}
	if _, ok, err := store.ConsumePairingCode(code.Code); err != nil || ok {
		t.Fatalf("expected pairing code to be one-time, ok=%v err=%v", ok, err)
	}
}

func TestAuthLogoutAPI_RevokeSessionAndClearsCookie(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	session, err := store.CreateSession(consoleauth.RoleUser, "iphone safari", time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	req.Host = "127.0.0.1:43180"
	req.AddCookie(&http.Cookie{Name: serverauth.DefaultBrowserSessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName)
	if cookie == nil || cookie.MaxAge >= 0 {
		t.Fatalf("expected clearing cookie, got %+v", cookie)
	}
	if cookie.Secure {
		t.Fatalf("loopback logout clearing cookie must not be Secure")
	}
	if _, ok, err := store.ValidateSession(session.ID); err != nil || ok {
		t.Fatalf("expected revoked session, ok=%v err=%v", ok, err)
	}
}

func TestAuthLogoutAPI_ClearsSecureCookieForHTTPSRequest(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	session, err := store.CreateSession(consoleauth.RoleUser, "safari", time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.0.2.10:5555"
	req.TLS = &tls.ConnectionState{}
	req.AddCookie(&http.Cookie{Name: serverauth.DefaultBrowserSessionCookieName, Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName)
	if cookie == nil || cookie.MaxAge >= 0 || !cookie.Secure {
		t.Fatalf("expected Secure clearing cookie for HTTPS logout, got %+v", cookie)
	}
}

func TestAuthPasswordAPI_AdminCanSetUserAndUserNeedsCurrentPassword(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	if err := store.SetPassword(consoleauth.RoleUser, "old user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)

	adminReq := httptest.NewRequest(http.MethodPatch, "/v1/auth/users/user/password", strings.NewReader(`{"new_password":"admin set user secret"}`))
	adminReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin password set 200, got %d body=%q", adminRec.Code, adminRec.Body.String())
	}
	if ok, err := store.VerifyPassword(consoleauth.RoleUser, "admin set user secret"); err != nil || !ok {
		t.Fatalf("expected admin-set user password to verify, ok=%v err=%v", ok, err)
	}

	userReq := httptest.NewRequest(http.MethodPatch, "/v1/auth/users/user/password", strings.NewReader(`{"current_password":"wrong","new_password":"new user secret"}`))
	userReq.Header.Set("Tars-Debug-Auth-Role", "user")
	userRec := httptest.NewRecorder()
	handler.ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected bad current password 401, got %d body=%q", userRec.Code, userRec.Body.String())
	}

	userReq = httptest.NewRequest(http.MethodPatch, "/v1/auth/users/user/password", strings.NewReader(`{"current_password":"admin set user secret","new_password":"new user secret"}`))
	userReq.Header.Set("Tars-Debug-Auth-Role", "user")
	userRec = httptest.NewRecorder()
	handler.ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusOK {
		t.Fatalf("expected user password change 200, got %d body=%q", userRec.Code, userRec.Body.String())
	}
	if ok, err := store.VerifyPassword(consoleauth.RoleUser, "new user secret"); err != nil || !ok {
		t.Fatalf("expected new user password to verify, ok=%v err=%v", ok, err)
	}
}

func TestAuthLoginAPI_RejectsInsecureNonLocalRequest(t *testing.T) {
	workspace := t.TempDir()
	if err := consoleauth.NewStore(workspace).SetPassword(consoleauth.RoleUser, "user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"user","password":"user secret"}`))
	req.Host = "example.com"
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected insecure non-local login to be forbidden, got %d body=%q", rec.Code, rec.Body.String())
	}
	if findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName) != nil {
		t.Fatalf("rejected login must not set a session cookie")
	}
}

func TestAuthLoginAPI_RejectsTailscaleHostWithoutIdentityHeader(t *testing.T) {
	workspace := t.TempDir()
	if err := consoleauth.NewStore(workspace).SetPassword(consoleauth.RoleUser, "user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"user","password":"user secret"}`))
	req.Host = "macbook.tailnet.ts.net"
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected Tailscale host without identity to be forbidden, got %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tagged device") {
		t.Fatalf("expected tagged-device guidance, got %q", rec.Body.String())
	}
	if findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName) != nil {
		t.Fatalf("rejected login must not set a session cookie")
	}
}

func TestAuthLoginAPI_RejectsSpoofedTailscaleHeaderFromNonLoopbackHTTP(t *testing.T) {
	workspace := t.TempDir()
	if err := consoleauth.NewStore(workspace).SetPassword(consoleauth.RoleUser, "user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"username":"user","password":"user secret"}`))
	req.Host = "example.com"
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set(serverauth.TailscaleUserLoginHeader, "user@example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected spoofed non-loopback Tailscale header to be forbidden, got %d body=%q", rec.Code, rec.Body.String())
	}
	if findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName) != nil {
		t.Fatalf("rejected login must not set a session cookie")
	}
}

func TestApplyAPIMiddleware_AllowsAuthLoginWithoutToken(t *testing.T) {
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: t.TempDir()},
		APIConfig:     config.APIConfig{APIAuthMode: "required", APIAdminToken: "admin-token"},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected login route to skip bearer middleware, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestApplyAPIMiddleware_AllowsPairingLoginWithoutToken(t *testing.T) {
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: t.TempDir()},
		APIConfig:     config.APIConfig{APIAuthMode: "required", APIAdminToken: "admin-token"},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/pairing-login", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected pairing login route to skip bearer middleware, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
