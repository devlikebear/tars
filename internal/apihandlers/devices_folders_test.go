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

type folderList struct {
	Folders []DeviceFolderView `json:"folders"`
}

func TestDevicesAPIFolders(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	h := NewDevicesHandler(store, zerolog.Nop())
	repo := t.TempDir()
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")

	rec := serveDevices(t, h, http.MethodGet, "/v1/admin/devices/folders", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"folders":[]`) {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{"folders": []string{repo, repo + string(filepath.Separator), missing, file}})
	rec = serveDevices(t, h, http.MethodPut, "/v1/admin/devices/folders", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	var got folderList
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []DeviceFolderView{
		{Path: filepath.Clean(repo), Name: filepath.Base(repo), Exists: true},
		{Path: missing, Name: "missing", Exists: false},
		{Path: file, Name: "file.txt", Exists: false},
	}
	if len(got.Folders) != len(want) {
		t.Fatalf("folders = %+v", got.Folders)
	}
	for i := range want {
		if got.Folders[i] != want[i] {
			t.Fatalf("folder %d = %+v, want %+v", i, got.Folders[i], want[i])
		}
	}

	rec = serveDevices(t, h, http.MethodGet, "/v1/admin/devices/folders", "")
	var listed folderList
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed.Folders) != 3 || !listed.Folders[0].Exists {
		t.Fatalf("list after put: %s", rec.Body.String())
	}
}

func TestDevicesAPIFoldersRejectsBadInput(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	h := NewDevicesHandler(store, zerolog.Nop())
	for _, folders := range [][]string{{"relative"}, {"  "}} {
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
	if _, err := DeviceFolderViews(store); err == nil {
		t.Fatalf("a broken file must be an error")
	}
}
