package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/rs/zerolog"
)

func TestAuthWhoamiAPI_ReturnsRoleAndWorkspace(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "required",
		APIUserToken:       "user-token",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), newAuthAPIHandler(cfg.APIAuthMode), io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/whoami", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Tars-Workspace-Id", "team-a")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool   `json:"authenticated"`
		AuthRole      string `json:"auth_role"`
		WorkspaceID   string `json:"workspace_id"`
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
	if body.WorkspaceID != "team-a" {
		t.Fatalf("expected workspace_id team-a, got %+v", body)
	}
	if body.AuthMode != "required" {
		t.Fatalf("expected auth_mode required, got %+v", body)
	}
	if body.IsAdmin {
		t.Fatalf("expected is_admin false, got %+v", body)
	}
}

func TestAuthWhoamiAPI_OffModeReturnsAnonymous(t *testing.T) {
	cfg := config.Config{
		APIAuthMode:        "off",
		APIWorkspaceHeader: "Tars-Workspace-Id",
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), newAuthAPIHandler(cfg.APIAuthMode), io.Discard)

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
		WorkspaceID   string `json:"workspace_id"`
		AuthMode      string `json:"auth_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode whoami response: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("expected authenticated=false, got %+v", body)
	}
	if body.AuthRole != "" {
		t.Fatalf("expected empty auth_role, got %+v", body)
	}
	if body.WorkspaceID != "default" {
		t.Fatalf("expected default workspace, got %+v", body)
	}
	if body.AuthMode != "off" {
		t.Fatalf("expected auth_mode off, got %+v", body)
	}
}
