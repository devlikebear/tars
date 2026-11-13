package apihandlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/rs/zerolog"
)

func TestDevicesAPIFolders(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	h := NewDevicesHandler(store, zerolog.Nop())
	repo := t.TempDir()

	rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/folders", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"folders":[]`) {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{"folders": []string{repo, repo + string(filepath.Separator)}})
	rec = serveDevices(t, h, http.MethodPut, "/v1/admin/devices/folders", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Folders []string `json:"folders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 1 || got.Folders[0] != filepath.Clean(repo) {
		t.Fatalf("stored folders = %v", got.Folders)
	}

	rec = serveDevices(t, h, http.MethodGet, "/v1/admin/devices/folders", "")
	if !strings.Contains(rec.Body.String(), filepath.Base(repo)) {
		t.Fatalf("list after put: %s", rec.Body.String())
	}
}

func TestDevicesAPIFoldersRejectsBadFolders(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	h := NewDevicesHandler(store, zerolog.Nop())
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, folders := range [][]string{
		{"relative"},
		{filepath.Join(t.TempDir(), "missing")},
		{file},
	} {
		body, _ := json.Marshal(map[string]any{"folders": folders})
		if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/folders", string(body)); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v: got %d", folders, rec.Code)
		}
	}
	many := make([]string, consoleauth.MaxDeviceFolders+1)
	for i := range many {
		many[i] = t.TempDir()
	}
	body, _ := json.Marshal(map[string]any{"folders": many})
	if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/folders", string(body)); rec.Code != http.StatusBadRequest {
		t.Fatalf("too many: got %d", rec.Code)
	}
	if rec := serveDevices(t, h, http.MethodPut, "/v1/admin/devices/folders", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body: got %d", rec.Code)
	}
	if got, _ := store.DeviceFolders(); len(got) != 0 {
		t.Fatalf("nothing may be stored: %v", got)
	}
}

func TestDevicesAPIFoldersUnreadable(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	h := NewDevicesHandler(store, zerolog.Nop())
	if err := os.MkdirAll(filepath.Join(workspace, "auth"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "auth", "device-folders.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/folders", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("broken file: got %d", rec.Code)
	}
}
