package apihandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/rs/zerolog"
)

func serveDevices(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDevicesAPIPairingCodeListAndRevoke(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	store := consoleauth.NewStore(t.TempDir(), consoleauth.WithNow(func() time.Time { return now }))
	h := NewDevicesHandler(store, zerolog.Nop())

	rec := serveDevices(t, h, http.MethodPost, "/v1/admin/devices/pairing-code", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pairing code: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("pairing code response must not be cached")
	}
	var code struct {
		Code      string    `json:"code"`
		Role      string    `json:"role"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &code); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code.Role != consoleauth.RoleFocus || len(code.Code) != 6 || !code.ExpiresAt.Equal(now.Add(DefaultDevicePairingTTL)) {
		t.Fatalf("unexpected pairing code: %+v", code)
	}
	if _, ok, err := store.ConsumePairingCodeForRole(code.Code, consoleauth.RoleFocus); err != nil || !ok {
		t.Fatalf("the code must pair a device: ok=%v err=%v", ok, err)
	}

	device, _, err := store.CreateDevice("phone", consoleauth.RoleFocus)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	rec = serveDevices(t, h, http.MethodGet, "/v1/admin/devices", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token_hash") || strings.Contains(rec.Body.String(), device.TokenHash) {
		t.Fatalf("list must not expose token hashes: %s", rec.Body.String())
	}
	var list struct {
		Devices []deviceView `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Devices) != 1 || list.Devices[0].ID != device.ID || !list.Devices[0].Active {
		t.Fatalf("unexpected list: %+v", list.Devices)
	}

	rec = serveDevices(t, h, http.MethodDelete, "/v1/admin/devices/"+device.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	rec = serveDevices(t, h, http.MethodGet, "/v1/admin/devices", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Devices[0].Active || list.Devices[0].RevokedAt == nil {
		t.Fatalf("device must show as revoked: %+v", list.Devices[0])
	}

	rec = serveDevices(t, h, http.MethodDelete, "/v1/admin/devices/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoke missing: %d", rec.Code)
	}
}

func TestDevicesAPIPairingCodeTTL(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	store := consoleauth.NewStore(t.TempDir(), consoleauth.WithNow(func() time.Time { return now }))
	h := NewDevicesHandler(store, zerolog.Nop())

	rec := serveDevices(t, h, http.MethodPost, "/v1/admin/devices/pairing-code", `{"ttl_seconds":120}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), now.Add(2*time.Minute).Format(time.RFC3339)) {
		t.Fatalf("ttl 120: %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{`{"ttl_seconds":-1}`, `{"ttl_seconds":3601}`} {
		if rec := serveDevices(t, h, http.MethodPost, "/v1/admin/devices/pairing-code", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d", body, rec.Code)
		}
	}
	if rec := serveDevices(t, h, http.MethodPost, "/v1/admin/devices/pairing-code", `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body: got %d", rec.Code)
	}
}

func TestDevicesAPIRoutesOnly(t *testing.T) {
	h := NewDevicesHandler(consoleauth.NewStore(t.TempDir()), zerolog.Nop())
	if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT list: got %d", rec.Code)
	}
	if rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/x/y", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path: got %d", rec.Code)
	}
}
