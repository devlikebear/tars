package serverauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func deviceResolver(tokens map[string]string) func(string) (string, bool) {
	return func(token string) (string, bool) {
		role, ok := tokens[token]
		return role, ok
	}
}

type seenRequest struct {
	role   string
	source string
}

func newDeviceTestHandler(t *testing.T, mode string, tokens map[string]string) (http.Handler, *seenRequest) {
	t.Helper()
	seen := &seenRequest{}
	mw := NewMiddleware(Options{
		Mode:                mode,
		UserToken:           "user-token",
		AdminToken:          "admin-token",
		AdminPaths:          []string{"/v1/admin/*"},
		DeviceTokenResolver: deviceResolver(tokens),
	}, io.Discard)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.role = RoleFromRequest(r)
		seen.source = AuthSourceFromRequest(r)
		w.WriteHeader(http.StatusNoContent)
	})), seen
}

func serveWithBearer(h http.Handler, method, path, remoteAddr, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDeviceTokenReachesOnlyFocusRoutes(t *testing.T) {
	h, seen := newDeviceTestHandler(t, ModeRequired, map[string]string{"phone-token": RoleFocus})
	const remote = "100.64.0.9:5555"

	rec := serveWithBearer(h, http.MethodGet, "/v1/focus/pipelines", remote, "phone-token")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("focus list: got %d body=%q", rec.Code, rec.Body.String())
	}
	if seen.role != RoleFocus || seen.source != authSourceDevice {
		t.Fatalf("handler saw role=%q source=%q", seen.role, seen.source)
	}

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/focus/pipelines/abc/gates/plan"},
		{http.MethodGet, "/v1/focus/pipelines/abc"},
		{http.MethodGet, "/v1/auth/whoami"},
		{http.MethodPost, "/v1/chat"},
		{http.MethodGet, "/v1/chat/stream"},
		{http.MethodPost, "/v1/chat/cancel"},
		{http.MethodPost, "/v1/chat/permissions/req-1"},
		{http.MethodGet, "/v1/chat/activity"},
		{http.MethodPost, "/v1/ops/approvals/a1/approve"},
		{http.MethodPost, "/v1/ops/approvals/a1/reject"},
	} {
		if rec := serveWithBearer(h, tc.method, tc.path, remote, "phone-token"); rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s: got %d", tc.method, tc.path, rec.Code)
		}
	}

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/status"},
		{http.MethodGet, "/v1/events/stream"},
		{http.MethodGet, "/v1/chat/board"},
		{http.MethodPost, "/v1/chat/permission-rules"},
		{http.MethodGet, "/v1/ops/approvals"},
		{http.MethodPost, "/v1/ops/approvals/a1/apply"},
		{http.MethodPost, "/v1/ops/cleanup/apply"},
		{http.MethodGet, "/v1/admin/sessions"},
		{http.MethodGet, "/v1/focus/exec-report"},
		{http.MethodPut, "/v1/focus/templates/dev"},
		{http.MethodGet, "/v1/memory/search"},
	} {
		if rec := serveWithBearer(h, tc.method, tc.path, remote, "phone-token"); rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: got %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestDeviceTokenTableAppliesOnLoopbackToo(t *testing.T) {
	// external-required lets loopback through without a token, but a request
	// that does present a device token is held to the device's table.
	h, _ := newDeviceTestHandler(t, ModeExternalRequired, map[string]string{"phone-token": RoleFocus})
	if rec := serveWithBearer(h, http.MethodGet, "/v1/status", "127.0.0.1:1234", "phone-token"); rec.Code != http.StatusForbidden {
		t.Fatalf("device token on loopback /v1/status: got %d, want 403", rec.Code)
	}
	if rec := serveWithBearer(h, http.MethodGet, "/v1/status", "127.0.0.1:1234", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("tokenless loopback must keep working: got %d", rec.Code)
	}
}

func TestDeviceResolverCannotGrantOtherRoles(t *testing.T) {
	h, _ := newDeviceTestHandler(t, ModeRequired, map[string]string{
		"claims-admin": RoleAdmin,
		"claims-user":  RoleUser,
	})
	for _, token := range []string{"claims-admin", "claims-user", "unknown"} {
		if rec := serveWithBearer(h, http.MethodGet, "/v1/status", "100.64.0.9:5555", token); rec.Code != http.StatusUnauthorized {
			t.Fatalf("token %q: got %d, want 401", token, rec.Code)
		}
	}
}

func TestStaticTokensWinOverDeviceResolver(t *testing.T) {
	// A device record that happens to carry the admin token's value must not
	// turn the admin token into a focus credential.
	h, seen := newDeviceTestHandler(t, ModeRequired, map[string]string{"admin-token": RoleFocus})
	rec := serveWithBearer(h, http.MethodGet, "/v1/admin/sessions", "100.64.0.9:5555", "admin-token")
	if rec.Code != http.StatusNoContent || seen.role != RoleAdmin || seen.source != authSourceBearer {
		t.Fatalf("admin token: code=%d role=%q source=%q", rec.Code, seen.role, seen.source)
	}
}

func TestFocusEndpointTable(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/v1/healthz"},
		{http.MethodGet, "/v1/auth/whoami"},
		{http.MethodGet, "/v1/focus/pipelines"},
		{http.MethodPost, "/v1/focus/pipelines"},
		{http.MethodGet, "/v1/focus/pipelines/s1"},
		{http.MethodHead, "/v1/focus/pipelines/s1"},
		{http.MethodPost, "/v1/focus/pipelines/s1/gates/merge"},
		{http.MethodPost, "/v1/focus/pipelines/s1/cards/c1"},
		{http.MethodPost, "/v1/focus/pipelines/s1/findings"},
		{http.MethodPost, "/v1/focus/pipelines/s1/advance"},
		{http.MethodPost, "/v1/focus/pipelines/s1/stop"},
		{http.MethodPost, "/v1/focus/pipelines/s1/qa"},
		{http.MethodPost, "/v1/focus/pipelines/s1/goal"},
		{http.MethodPost, "/v1/focus/pipelines/s1/plan?x=1"},
		{http.MethodGet, "/v1/focus/templates"},
		{http.MethodGet, "/v1/focus/pipelines/s1/session"},
		{http.MethodGet, "/v1/focus/pipelines/s1/history?session=qa"},
		{http.MethodGet, "/v1/focus/pipelines/s1/checkpoints"},
		{http.MethodGet, "/v1/focus/pipelines/s1/checkpoints/t1/diff?scope=turn"},
	}
	for _, tc := range allowed {
		if !EndpointAllowsRole(tc.method, tc.path, RoleFocus) {
			t.Errorf("focus must reach %s %s", tc.method, tc.path)
		}
	}
	denied := []struct{ method, path string }{
		{http.MethodDelete, "/v1/focus/pipelines/s1"},
		{http.MethodGet, "/v1/focus/pipelines/s1/gates/merge"},
		{http.MethodPost, "/v1/focus/pipelines/s1/gates"},
		{http.MethodGet, "/v1/focus/release-train"},
		{http.MethodPost, "/v1/focus/templates/draft"},
		{http.MethodGet, "/v1/focus/exec-report"},
		{http.MethodGet, "/v1/setup/status"},
		{http.MethodPost, "/v1/auth/logout"},
		{http.MethodGet, "/v1/chat/board"},
		// The pipeline screen's reads come only through the focus routes.
		{http.MethodGet, "/v1/admin/sessions/s1"},
		{http.MethodGet, "/v1/admin/sessions/s1/history"},
		{http.MethodGet, "/v1/admin/sessions/s1/checkpoints"},
		{http.MethodPost, "/v1/focus/pipelines/s1/checkpoints/t1/revert"},
		{http.MethodPost, "/v1/focus/pipelines/s1/history"},
	}
	for _, tc := range denied {
		if EndpointAllowsRole(tc.method, tc.path, RoleFocus) {
			t.Errorf("focus must not reach %s %s", tc.method, tc.path)
		}
	}
	for _, rule := range focusEndpointRules {
		if rule.Access != EndpointAccessFocus && rule.Access != EndpointAccessPublic {
			t.Errorf("rule %s has access %q", rule.Pattern, rule.Access)
		}
	}
}

func TestUnknownRoleReachesNothing(t *testing.T) {
	if EndpointAllowsRole(http.MethodGet, "/v1/healthz", "robot") {
		t.Fatalf("an unknown role must reach nothing")
	}
}
