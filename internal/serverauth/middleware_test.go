package serverauth

import (
	"context"
	"encoding/json"
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
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "unauthorized" {
		t.Fatalf("expected code unauthorized, got %q", body["code"])
	}
	if body["error"] != "unauthorized" {
		t.Fatalf("expected error unauthorized, got %q", body["error"])
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

func TestMiddleware_LoopbackSkipPaths_SkipOnlyAppliesToLoopbackRequests(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:              ModeRequired,
		BearerToken:       "dev-token",
		LoopbackSkipPaths: []string{"/dashboards", "/ui/projects/*"},
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	loopbackReq := httptest.NewRequest(http.MethodGet, "/ui/projects/demo", nil)
	loopbackReq.RemoteAddr = "127.0.0.1:1234"
	loopbackRec := httptest.NewRecorder()
	h.ServeHTTP(loopbackRec, loopbackReq)
	if loopbackRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for loopback skip path, got %d body=%q", loopbackRec.Code, loopbackRec.Body.String())
	}

	externalReq := httptest.NewRequest(http.MethodGet, "/ui/projects/demo", nil)
	externalReq.RemoteAddr = "192.0.2.44:5555"
	externalRec := httptest.NewRecorder()
	h.ServeHTTP(externalRec, externalReq)
	if externalRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for external request on loopback-only skip path, got %d body=%q", externalRec.Code, externalRec.Body.String())
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

func TestWithWorkspaceID(t *testing.T) {
	ctx := WithWorkspaceID(context.Background(), "ws-test")
	if got := WorkspaceIDFromContext(ctx); got != "ws-test" {
		t.Fatalf("expected ws-test, got %q", got)
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
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "forbidden" {
		t.Fatalf("expected code forbidden, got %q", body["code"])
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

func TestMiddleware_AdminPathWildcardRejectsUserToken(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:       ModeRequired,
		UserToken:  "user-token",
		AdminToken: "admin-token",
		AdminPaths: []string{"/v1/channels/webhook/inbound/*"},
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	reqUser := httptest.NewRequest(http.MethodPost, "/v1/channels/webhook/inbound/general", nil)
	reqUser.RemoteAddr = "127.0.0.1:1234"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wildcard admin path with user token, got %d body=%q", recUser.Code, recUser.Body.String())
	}

	reqAdmin := httptest.NewRequest(http.MethodPost, "/v1/channels/webhook/inbound/general", nil)
	reqAdmin.RemoteAddr = "127.0.0.1:1234"
	reqAdmin.Header.Set("Authorization", "Bearer admin-token")
	recAdmin := httptest.NewRecorder()
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for wildcard admin path with admin token, got %d body=%q", recAdmin.Code, recAdmin.Body.String())
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
		Mode:                          ModeRequired,
		UserToken:                     "user-token",
		WorkspaceHeader:               "Tars-Workspace-Id",
		RequireWorkspaceForAuthorized: true,
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
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "workspace_id_required" {
		t.Fatalf("expected code workspace_id_required, got %q", body["code"])
	}
}

func TestMiddleware_RequireWorkspaceForAuthenticatedRequest_AllowsWithHeader(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:                          ModeRequired,
		UserToken:                     "user-token",
		WorkspaceHeader:               "Tars-Workspace-Id",
		RequireWorkspaceForAuthorized: true,
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

func TestMiddleware_WorkspaceAllowlistByRole(t *testing.T) {
	mw := NewMiddleware(Options{
		Mode:                          ModeRequired,
		UserToken:                     "user-token",
		AdminToken:                    "admin-token",
		WorkspaceHeader:               "Tars-Workspace-Id",
		RequireWorkspaceForAuthorized: true,
		UserWorkspaceAllowlist:        []string{"ws-user"},
		AdminWorkspaceAllowlist:       []string{"ws-admin"},
	}, io.Discard)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	reqUser := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	reqUser.RemoteAddr = "127.0.0.1:1234"
	reqUser.Header.Set("Authorization", "Bearer user-token")
	reqUser.Header.Set("Tars-Workspace-Id", "ws-other")
	recUser := httptest.NewRecorder()
	h.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected user to be forbidden for workspace mismatch, got %d body=%q", recUser.Code, recUser.Body.String())
	}

	reqAdmin := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	reqAdmin.RemoteAddr = "127.0.0.1:1234"
	reqAdmin.Header.Set("Authorization", "Bearer admin-token")
	reqAdmin.Header.Set("Tars-Workspace-Id", "ws-admin")
	recAdmin := httptest.NewRecorder()
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusNoContent {
		t.Fatalf("expected admin to pass for allowed workspace, got %d body=%q", recAdmin.Code, recAdmin.Body.String())
	}
}

func TestCompileOptions_NormalizesPathsTokensAndWorkspaceSettings(t *testing.T) {
	compiled := compileOptions(Options{
		Mode:                          " ExTerNal-ReQuired ",
		UserToken:                     "user-token",
		AdminToken:                    "admin-token",
		RequireWorkspaceForAuthorized: true,
		SkipPaths:                     []string{" /healthz ", "", "/ready"},
		AdminPaths:                    []string{" /v1/admin/reload ", " /v1/channels/webhook/inbound/* ", ""},
		UserWorkspaceAllowlist:        []string{" ws-user ", ""},
		AdminWorkspaceAllowlist:       []string{" ws-admin "},
	}, io.Discard)

	if compiled.mode != ModeExternalRequired {
		t.Fatalf("expected normalized mode %q, got %q", ModeExternalRequired, compiled.mode)
	}
	if compiled.workspaceHeader != DefaultWorkspaceHeader {
		t.Fatalf("expected default workspace header %q, got %q", DefaultWorkspaceHeader, compiled.workspaceHeader)
	}
	if !compiled.skipPaths.match("/healthz") || !compiled.skipPaths.match("/ready") {
		t.Fatalf("expected skip paths to include trimmed entries")
	}
	if !compiled.adminPaths.match("/v1/admin/reload") {
		t.Fatalf("expected exact admin path to match")
	}
	if !compiled.adminPaths.match("/v1/channels/webhook/inbound/general") {
		t.Fatalf("expected wildcard admin path to match by prefix")
	}
	if compiled.resolveRole("Bearer user-token") != RoleUser {
		t.Fatalf("expected user token to resolve to role user")
	}
	if compiled.resolveRole("Bearer admin-token") != RoleAdmin {
		t.Fatalf("expected admin token to resolve to role admin")
	}
	if compiled.resolveRole("Bearer unknown-token") != "" {
		t.Fatalf("expected unknown token to resolve to empty role")
	}
	if !compiled.requireWorkspaceForAuthorized {
		t.Fatalf("expected requireWorkspaceForAuthorized to be preserved")
	}
	if !isWorkspaceAllowed(compiled.userWorkspaceAllowlist, "ws-user") {
		t.Fatalf("expected user workspace allowlist to contain trimmed workspace")
	}
	if !isWorkspaceAllowed(compiled.adminWorkspaceAllowlist, "ws-admin") {
		t.Fatalf("expected admin workspace allowlist to contain trimmed workspace")
	}
}

func TestCompiledOptions_RequirementForRequest(t *testing.T) {
	compiled := compileOptions(Options{
		Mode:       ModeExternalRequired,
		UserToken:  "user-token",
		AdminToken: "admin-token",
		SkipPaths:  []string{"/v1/status"},
		AdminPaths: []string{"/v1/admin/*"},
	}, io.Discard)

	cases := []struct {
		name            string
		path            string
		remoteAddr      string
		wantSkip        bool
		wantRequireAuth bool
		wantAdminPath   bool
		wantTokenNeeded bool
	}{
		{
			name:       "skip path bypasses auth",
			path:       "/v1/status",
			remoteAddr: "192.0.2.10:443",
			wantSkip:   true,
		},
		{
			name:            "loopback request in external required mode is optional",
			path:            "/v1/chat",
			remoteAddr:      "127.0.0.1:8080",
			wantRequireAuth: false,
			wantAdminPath:   false,
			wantTokenNeeded: false,
		},
		{
			name:            "ipv6 loopback request in external required mode is optional",
			path:            "/v1/chat",
			remoteAddr:      "[::1]:8080",
			wantRequireAuth: false,
			wantAdminPath:   false,
			wantTokenNeeded: false,
		},
		{
			name:            "loopback alias request in external required mode is optional",
			path:            "/v1/chat",
			remoteAddr:      "127.0.0.2:8080",
			wantRequireAuth: false,
			wantAdminPath:   false,
			wantTokenNeeded: false,
		},
		{
			name:            "external request in external required mode needs token",
			path:            "/v1/chat",
			remoteAddr:      "192.0.2.10:443",
			wantRequireAuth: true,
			wantAdminPath:   false,
			wantTokenNeeded: true,
		},
		{
			name:            "admin path on loopback still needs token",
			path:            "/v1/admin/reload",
			remoteAddr:      "127.0.0.1:8080",
			wantRequireAuth: false,
			wantAdminPath:   true,
			wantTokenNeeded: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.RemoteAddr = tc.remoteAddr

			got := compiled.requirementForRequest(req)

			if got.skip != tc.wantSkip {
				t.Fatalf("expected skip=%v, got %v", tc.wantSkip, got.skip)
			}
			if got.requireToken != tc.wantRequireAuth {
				t.Fatalf("expected requireToken=%v, got %v", tc.wantRequireAuth, got.requireToken)
			}
			if got.isAdminPath != tc.wantAdminPath {
				t.Fatalf("expected isAdminPath=%v, got %v", tc.wantAdminPath, got.isAdminPath)
			}
			if got.tokenNeeded != tc.wantTokenNeeded {
				t.Fatalf("expected tokenNeeded=%v, got %v", tc.wantTokenNeeded, got.tokenNeeded)
			}
		})
	}
}

func TestIsLoopbackRemoteAddr(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:8080", want: true},
		{name: "ipv4 loopback alias", remoteAddr: "127.0.0.2:8080", want: true},
		{name: "ipv6 loopback", remoteAddr: "[::1]:8080", want: true},
		{name: "ipv6 loopback zone suffix", remoteAddr: "[::1%lo0]:8080", want: true},
		{name: "external ipv4", remoteAddr: "192.0.2.10:443", want: false},
		{name: "empty", remoteAddr: "", want: false},
		{name: "host name", remoteAddr: "localhost:8080", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLoopbackRemoteAddr(tc.remoteAddr); got != tc.want {
				t.Fatalf("expected %v, got %v for %q", tc.want, got, tc.remoteAddr)
			}
		})
	}
}
