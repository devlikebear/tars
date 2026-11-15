package apihandlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/rs/zerolog"
)

// fakeKeepAwake reads the stored choice and counts how often it was asked
// to act on it.
type fakeKeepAwake struct {
	store   *consoleauth.Store
	applied int
	err     error
}

func (f *fakeKeepAwake) KeepAwakeStatus() (KeepAwakeStatus, error) {
	if f.err != nil {
		return KeepAwakeStatus{}, f.err
	}
	on, err := f.store.KeepAwake()
	return KeepAwakeStatus{Enabled: on, Supported: true, Active: on}, err
}

func (f *fakeKeepAwake) ApplyKeepAwake() { f.applied++ }

func decodeKeepAwake(t *testing.T, body []byte) KeepAwakeStatus {
	t.Helper()
	var s KeepAwakeStatus
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDevicesAPIKeepAwake(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	control := &fakeKeepAwake{store: store}
	h := NewDevicesHandler(store, zerolog.Nop(), WithKeepAwake(control))

	rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/keep-awake", "")
	if rec.Code != http.StatusOK || decodeKeepAwake(t, rec.Body.Bytes()) != (KeepAwakeStatus{Supported: true}) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	rec = serveDevices(t, h, http.MethodPut, "/v1/admin/devices/keep-awake", `{"enabled":true}`)
	if rec.Code != http.StatusOK || decodeKeepAwake(t, rec.Body.Bytes()) != (KeepAwakeStatus{Enabled: true, Supported: true, Active: true}) {
		t.Fatalf("put on: %d %s", rec.Code, rec.Body.String())
	}
	if control.applied != 1 {
		t.Fatalf("the loop must act at once: applied %d", control.applied)
	}
	if on, _ := store.KeepAwake(); !on {
		t.Fatal("not stored")
	}

	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `nope`} {
		if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/keep-awake", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	if control.applied != 1 {
		t.Fatal("a refused request must not act")
	}

	control.err = errors.New("devices unreadable")
	if rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/keep-awake", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status error: %d", rec.Code)
	}
}

func TestDevicesAPIKeepAwakeWithoutTheLoop(t *testing.T) {
	dir := t.TempDir()
	store := consoleauth.NewStore(dir)
	h := NewDevicesHandler(store, zerolog.Nop())
	rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/keep-awake", `{"enabled":true}`)
	if rec.Code != http.StatusOK || decodeKeepAwake(t, rec.Body.Bytes()) != (KeepAwakeStatus{Enabled: true, Reason: "unsupported"}) {
		t.Fatalf("no loop: %d %s", rec.Code, rec.Body.String())
	}

	// The choice can be neither saved nor read: a folder stands where the
	// file goes.
	blocked := consoleauth.NewStore(filepath.Join(dir, "blocked"))
	if err := os.MkdirAll(filepath.Join(dir, "blocked", "auth", "keep-awake.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	h = NewDevicesHandler(blocked, zerolog.Nop())
	if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/keep-awake", `{"enabled":true}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("unsavable: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/keep-awake", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("unreadable: %d %s", rec.Code, rec.Body.String())
	}
}
