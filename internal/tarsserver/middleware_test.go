package tarsserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestApplyAPIMiddleware_RejectsExternalWithoutToken(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:  "external-required",
		APIAuthToken: "dev-token",
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

func TestApplyAPIMiddleware_AllowsExternalWithTokenAndBindsDefaultWorkspace(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:  "external-required",
		APIAuthToken: "dev-token",
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
	if got := strings.TrimSpace(rec.Body.String()); got != defaultWorkspaceID {
		t.Fatalf("expected workspace id %q, got %q", defaultWorkspaceID, got)
	}
}

func TestApplyAPIMiddleware_AdminPathRequiresAdminRole(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:   "required",
		APIUserToken:  "user-token",
		APIAdminToken: "admin-token",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	reqUser := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	reqUser.RemoteAddr = "192.0.2.10:5555"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user token on admin path, got %d body=%q", recUser.Code, recUser.Body.String())
	}

	reqAdmin := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	reqAdmin.RemoteAddr = "192.0.2.10:5555"
	reqAdmin.Header.Set("Authorization", "Bearer admin-token")
	recAdmin := httptest.NewRecorder()
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for admin token on admin path, got %d body=%q", recAdmin.Code, recAdmin.Body.String())
	}
}

func TestMiddleware_AdminPaths_TelegramSendIsNotAdminOnly(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:   "required",
		APIUserToken:  "user-token",
		APIAdminToken: "admin-token",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	reqUser := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/send", nil)
	reqUser.RemoteAddr = "192.0.2.10:5555"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for user token on telegram send path, got %d body=%q", recUser.Code, recUser.Body.String())
	}
}

func TestMiddleware_AdminPaths_TelegramPairingsAreAdminOnly(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:   "required",
		APIUserToken:  "user-token",
		APIAdminToken: "admin-token",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	reqUser := httptest.NewRequest(http.MethodGet, "/v1/channels/telegram/pairings", nil)
	reqUser.RemoteAddr = "192.0.2.10:5555"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user token on telegram pairings path, got %d body=%q", recUser.Code, recUser.Body.String())
	}

	reqAdmin := httptest.NewRequest(http.MethodGet, "/v1/channels/telegram/pairings", nil)
	reqAdmin.RemoteAddr = "192.0.2.10:5555"
	reqAdmin.Header.Set("Authorization", "Bearer admin-token")
	recAdmin := httptest.NewRecorder()
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for admin token on telegram pairings path, got %d body=%q", recAdmin.Code, recAdmin.Body.String())
	}
}

func TestApplyAPIMiddleware_StatusIncludesAuthMetadataSingleWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	if _, err := store.Create("status test"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	mainSession, err := resolveMainSessionID(store, "")
	if err != nil {
		t.Fatalf("resolve main session: %v", err)
	}
	statusHandler := newStatusAPIHandler(root, store, mainSession, zerolog.New(io.Discard))
	cfg := config.Config{
		APIAuthMode:   "required",
		APIUserToken:  "user-token",
		APIAdminToken: "admin-token",
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
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if got := strings.TrimSpace(asString(body["workspace_dir"])); got != root {
		t.Fatalf("expected workspace_dir %q, got %q", root, got)
	}
	if got := intFromAny(body["session_count"]); got != 2 {
		t.Fatalf("expected session_count 2, got %d", got)
	}
	if got := strings.TrimSpace(asString(body["auth_role"])); got != "user" {
		t.Fatalf("expected auth_role user, got %q", got)
	}
	if _, ok := body["workspace_id"]; ok {
		t.Fatalf("workspace_id must be removed from status response: %+v", body)
	}
}

func TestApplyAPIMiddleware_DebugLogIncludesRoleOnly(t *testing.T) {
	var logs bytes.Buffer
	cfg := config.Config{
		APIAuthMode:   "required",
		APIUserToken:  "user-token",
		APIAdminToken: "admin-token",
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
	if strings.Contains(line, `"workspace_id":`) {
		t.Fatalf("workspace_id must be removed from debug log, got %q", line)
	}
	if !strings.Contains(line, `"auth_role":"user"`) {
		t.Fatalf("expected debug log to include auth_role, got %q", line)
	}
}

func intFromAny(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}
