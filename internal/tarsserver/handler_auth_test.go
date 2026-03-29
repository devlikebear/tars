package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestAuthWhoamiAPI_ReturnsRole(t *testing.T) {
	cfg := config.Config{
		APIConfig: config.APIConfig{
			APIAuthMode:  "required",
			APIUserToken: "user-token",
		},
	}
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), newAuthAPIHandler(cfg.APIAuthMode), io.Discard)

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
	handler := newAuthAPIHandler("required")
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
