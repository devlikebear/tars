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
