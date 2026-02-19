package serverauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_ExternalRequired_AllowsLoopbackWithoutToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeExternalRequired,
		BearerToken: "dev-token",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_ExternalRequired_RejectsExternalWithoutToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeExternalRequired,
		BearerToken: "dev-token",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.44:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_ExternalRequired_AllowsExternalWithToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeExternalRequired,
		BearerToken: "dev-token",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.44:5555"
	req.Header.Set("Authorization", "Bearer dev-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_Required_RejectsLoopbackWithoutToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeRequired,
		BearerToken: "dev-token",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_Off_AllowsAnyRequest(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeOff,
		BearerToken: "dev-token",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "203.0.113.10:443"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_SetsWorkspaceIDFromHeader(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:            ModeOff,
		WorkspaceHeader: "Tars-Workspace-Id",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(WorkspaceIDFromContext(r.Context())))
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Tars-Workspace-Id", "ws-dev")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "ws-dev" {
		t.Fatalf("expected workspace id ws-dev, got %q", got)
	}
}

func TestMiddleware_AdminPathRejectsUserToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeRequired,
		UserToken:   "user-token",
		AdminToken:  "admin-token",
		AdminPaths:  []string{"/v1/gateway/reload"},
		BearerToken: "",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_AdminPathAllowsAdminToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeRequired,
		UserToken:   "user-token",
		AdminToken:  "admin-token",
		AdminPaths:  []string{"/v1/gateway/reload"},
		BearerToken: "",
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RoleFromContext(r.Context())))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != RoleAdmin {
		t.Fatalf("expected role admin, got %q", got)
	}
}

func TestMiddleware_BackwardCompatibleSingleTokenAllowsAdminPath(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:        ModeRequired,
		BearerToken: "legacy-token",
		AdminPaths:  []string{"/v1/gateway/reload"},
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RoleFromContext(r.Context())))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer legacy-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != RoleAdmin {
		t.Fatalf("expected role admin for legacy token, got %q", got)
	}
}

func TestMiddleware_AdminPathWithoutConfiguredTokenReturnsUnauthorized(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:       ModeExternalRequired,
		AdminPaths: []string{"/v1/sentinel/restart"},
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/sentinel/restart", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("expected WWW-Authenticate Bearer, got %q", got)
	}
}

func TestMiddleware_RequireWorkspaceForAuthenticatedRequest(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:                           ModeRequired,
		UserToken:                      "user-token",
		WorkspaceHeader:                "Tars-Workspace-Id",
		RequireWorkspaceForAuthorized:  true,
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_RequireWorkspaceForAuthenticatedRequest_AllowsWithHeader(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:                           ModeRequired,
		UserToken:                      "user-token",
		WorkspaceHeader:                "Tars-Workspace-Id",
		RequireWorkspaceForAuthorized:  true,
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Tars-Workspace-Id", "ws-main")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%q", rec.Code, rec.Body.String())
	}
}
