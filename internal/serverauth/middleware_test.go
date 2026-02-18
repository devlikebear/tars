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
