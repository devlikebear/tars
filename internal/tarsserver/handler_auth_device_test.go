package tarsserver

import (
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

func devicePairRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/device-pair", strings.NewReader(body))
	req.Host = "macbook.tailnet.ts.net"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(serverauth.TailscaleUserLoginHeader, "user@example.com")
	return req
}

func TestDevicePairAPI_TradesFocusCodeForToken(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	code, err := store.CreatePairingCode(consoleauth.RoleFocus, time.Minute)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, devicePairRequest(`{"code":"`+code.Code+`","name":"iPhone"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("token response must not be cached")
	}
	if cookie := findCookie(rec.Result().Cookies(), serverauth.DefaultBrowserSessionCookieName); cookie != nil {
		t.Fatalf("device pairing must not set a browser cookie")
	}
	var body struct {
		DeviceID string `json:"device_id"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Token    string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Role != consoleauth.RoleFocus || body.Name != "iPhone" || body.Token == "" || body.DeviceID == "" {
		t.Fatalf("unexpected body: %+v", body)
	}
	device, ok, err := store.ValidateDeviceToken(body.Token)
	if err != nil || !ok || device.ID != body.DeviceID {
		t.Fatalf("token must validate: %+v %v %v", device, ok, err)
	}

	// The code is single-use.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, devicePairRequest(`{"code":"`+code.Code+`","name":"iPhone"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused code: expected 401, got %d", rec.Code)
	}
}

func TestDevicePairAPI_RejectsBrowserCodeWithoutBurningIt(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	code, err := store.CreatePairingCode(consoleauth.RoleUser, time.Minute)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	handler := newAuthAPIHandler("required", workspace)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, devicePairRequest(`{"code":"`+code.Code+`","name":"iPhone"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
	if _, ok, err := store.ConsumePairingCode(code.Code); err != nil || !ok {
		t.Fatalf("the browser code must still work: ok=%v err=%v", ok, err)
	}
	if devices, _ := store.ListDevices(); len(devices) != 0 {
		t.Fatalf("no device may be created: %+v", devices)
	}
}

func TestDevicePairAPI_RejectsBadRequests(t *testing.T) {
	workspace := t.TempDir()
	handler := newAuthAPIHandler("required", workspace)

	insecure := httptest.NewRequest(http.MethodPost, "/v1/auth/device-pair", strings.NewReader(`{"code":"123456","name":"x"}`))
	insecure.Host = "192.0.2.10:43180"
	insecure.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, insecure)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain HTTP from another host: expected 403, got %d", rec.Code)
	}

	for _, body := range []string{`{`, `{"code":"123456"}`, `{"code":"123456","name":"  "}`} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, devicePairRequest(body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", body, rec.Code)
		}
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/device-pair", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: expected 405, got %d", rec.Code)
	}
}

func TestDevicePairAPI_LocksAfterRepeatedFailures(t *testing.T) {
	handler := newAuthAPIHandler("required", t.TempDir())
	var last int
	for i := 0; i < 6; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, devicePairRequest(`{"code":"000000","name":"iPhone"}`))
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
}

func TestApplyAPIMiddleware_DeviceTokenReachesFocusOnly(t *testing.T) {
	workspace := t.TempDir()
	_, token, err := consoleauth.NewStore(workspace).CreateDevice("iPhone", consoleauth.RoleFocus)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
		APIConfig:     config.APIConfig{APIAuthMode: "required", APIAdminToken: "admin-token"},
	}
	var seenRole string
	h := applyAPIMiddleware(cfg, zerolog.New(io.Discard), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRole = serverauth.RoleFromRequest(r)
		w.WriteHeader(http.StatusNoContent)
	}), io.Discard)

	serve := func(method, path, bearer string) int {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "100.64.0.9:5555"
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := serve(http.MethodGet, "/v1/focus/pipelines", token); code != http.StatusNoContent || seenRole != serverauth.RoleFocus {
		t.Fatalf("focus list: code=%d role=%q", code, seenRole)
	}
	if code := serve(http.MethodGet, "/v1/admin/devices", token); code != http.StatusForbidden {
		t.Fatalf("admin devices with a device token: got %d, want 403", code)
	}
	if code := serve(http.MethodPost, "/v1/chat", token); code != http.StatusForbidden {
		t.Fatalf("chat with a device token: got %d, want 403", code)
	}
	if code := serve(http.MethodPost, "/v1/auth/device-pair", ""); code != http.StatusNoContent {
		t.Fatalf("device-pair must skip bearer auth: got %d", code)
	}
}
