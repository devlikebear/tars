package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
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

func TestApplyAPIMiddleware_AdminPathRequiresAdminRole(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIAdminToken:      "admin-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	reqUser := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	reqUser.RemoteAddr = "192.0.2.10:5555"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	reqUser.Header.Set("Tars-Workspace-Id", "ws-local")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user token on admin path, got %d body=%q", recUser.Code, recUser.Body.String())
	}

	reqAdmin := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	reqAdmin.RemoteAddr = "192.0.2.10:5555"
	reqAdmin.Header.Set("Authorization", "Bearer admin-token")
	reqAdmin.Header.Set("Tars-Workspace-Id", "ws-local")
	recAdmin := httptest.NewRecorder()
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for admin token on admin path, got %d body=%q", recAdmin.Code, recAdmin.Body.String())
	}
}

func TestApplyAPIMiddleware_DebugLogIncludesWorkspaceAndRole(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIAdminToken:      "admin-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	h := applyAPIMiddleware(cfg, logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Tars-Workspace-Id", "ws-local")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%q", rec.Code, rec.Body.String())
	}
	line := logs.String()
	if !strings.Contains(line, `"workspace_id":"ws-local"`) {
		t.Fatalf("expected debug log to include workspace_id, got %q", line)
	}
	if !strings.Contains(line, `"auth_role":"user"`) {
		t.Fatalf("expected debug log to include auth_role, got %q", line)
	}
}

func TestApplyAPIMiddleware_ForbiddenAdminPathIncludesUserRoleInDebugLog(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIAdminToken:      "admin-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	h := applyAPIMiddleware(cfg, logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Tars-Workspace-Id", "ws-local")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), `"auth_role":"user"`) {
		t.Fatalf("expected debug log to include auth_role=user, got %q", logs.String())
	}
}

func TestApplyAPIMiddleware_StatusIncludesAuthMetadata(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	if _, err := store.Create("status test"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	statusHandler := newStatusAPIHandler(root, store, zerolog.New(io.Discard))
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIAdminToken:      "admin-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), statusHandler, io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Tars-Workspace-Id", "ws-local")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		WorkspaceDir string `json:"workspace_dir"`
		SessionCount int    `json:"session_count"`
		WorkspaceID  string `json:"workspace_id"`
		AuthRole     string `json:"auth_role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	expectedWorkspaceDir := filepath.Join(root, "_workspaces", "ws-local")
	if body.WorkspaceDir != expectedWorkspaceDir {
		t.Fatalf("expected workspace_dir %q, got %q", expectedWorkspaceDir, body.WorkspaceDir)
	}
	if body.SessionCount != 0 {
		t.Fatalf("expected session_count 0 for workspace ws-local, got %d", body.SessionCount)
	}
	if body.WorkspaceID != "ws-local" {
		t.Fatalf("expected workspace_id ws-local, got %q", body.WorkspaceID)
	}
	if body.AuthRole != "user" {
		t.Fatalf("expected auth_role user, got %q", body.AuthRole)
	}
}

func TestApplyAPIMiddleware_AuthenticatedRequestRequiresWorkspaceHeader(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace header, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestApplyAPIMiddleware_RejectsWorkspaceOutsideRoleAllowlist(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:          "required",
		APIUserToken:         "user-token",
		APIAdminToken:        "admin-token",
		APIWorkspaceHeader:   "Tars-Workspace-Id",
		APIUserWorkspaceIDs:  []string{"ws-user"},
		APIAdminWorkspaceIDs: []string{"ws-admin"},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	reqUser := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	reqUser.RemoteAddr = "192.0.2.10:5555"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	reqUser.Header.Set("Tars-Workspace-Id", "ws-other")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user workspace mismatch, got %d body=%q", recUser.Code, recUser.Body.String())
	}
}
