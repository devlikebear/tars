package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/rs/zerolog"
)

func TestApplyAPIMiddleware_RejectsExternalWithoutToken(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "external-required",
		APIAuthToken:       "dev-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestApplyAPIMiddleware_AllowsExternalWithTokenAndWorkspaceHeader(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "external-required",
		APIAuthToken:       "dev-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(serverauth.WorkspaceIDFromContext(r.Context())))
	}), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer dev-token")
	req.Header.Set("Tars-Workspace-Id", "ws-local")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "ws-local" {
		t.Fatalf("expected workspace id ws-local, got %q", got)
	}
}

func TestApplyAPIMiddleware_HealthzBypassesAuth(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "required",
		APIAuthToken:       "dev-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for /v1/healthz bypass, got %d body=%q", rec.Code, rec.Body.String())
	}
}
