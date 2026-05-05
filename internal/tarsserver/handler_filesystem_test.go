package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestFilesystemBrowseHandler_IncludesTarsDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".tars", ".git", "node_modules", "visible"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	handler := newFilesystemBrowseHandler(zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/v1/filesystem/browse?path="+filepath.ToSlash(root), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	names := map[string]bool{}
	for _, entry := range got.Entries {
		names[entry.Name] = true
	}
	if !names[".tars"] || !names["visible"] {
		t.Fatalf("expected .tars and visible directories, got %+v", got.Entries)
	}
	for _, hidden := range []string{".git", "node_modules"} {
		if names[hidden] {
			t.Fatalf("did not expect hidden directory %s in %+v", hidden, got.Entries)
		}
	}
}

func TestFilesystemBrowseHandler_CreatesDirectory(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "projects")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatalf("mkdir parent dir: %v", err)
	}

	handler := newFilesystemBrowseHandler(zerolog.New(io.Discard))
	body := strings.NewReader(`{"parent_path":"` + filepath.ToSlash(parentDir) + `","name":"notes"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/filesystem/browse", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "notes" || !got.IsDir {
		t.Fatalf("unexpected response: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(parentDir, "notes")); err != nil {
		t.Fatalf("expected created directory: %v", err)
	}
}

func TestFilesystemBrowseHandler_RejectsRelativeParentPathOnCreate(t *testing.T) {
	handler := newFilesystemBrowseHandler(zerolog.New(io.Discard))
	body := strings.NewReader(`{"parent_path":"projects","name":"notes"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/filesystem/browse", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
}
