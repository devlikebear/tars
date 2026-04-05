package tarsserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

type workspaceFilePreviewResponse struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	UpdatedAt   string `json:"updated_at"`
	Kind        string `json:"kind"`
	MIMEType    string `json:"mime_type"`
	Encoding    string `json:"encoding,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentBase string `json:"content_base64,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	IsBinary    bool   `json:"is_binary,omitempty"`
	Message     string `json:"message,omitempty"`
}

func TestWorkspaceFilesHandler_ReadsMarkdownPreview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")
	if err := os.WriteFile(filepath.Join(artifactsRoot, "note.md"), []byte("# Title\n\nhello"), 0o644); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/?path=note.md&root="+url.QueryEscape(artifactsRoot), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got workspaceFilePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Kind != "markdown" {
		t.Fatalf("expected markdown kind, got %q", got.Kind)
	}
	if !strings.Contains(got.Content, "# Title") {
		t.Fatalf("expected markdown content, got %q", got.Content)
	}
	if got.Encoding != "utf-8" {
		t.Fatalf("expected utf-8 encoding, got %q", got.Encoding)
	}
}

func TestWorkspaceFilesHandler_ReadsImagePreviewAsBase64(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7Z0ioAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsRoot, "pixel.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("write image file: %v", err)
	}

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/?path=pixel.png&root="+url.QueryEscape(artifactsRoot), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got workspaceFilePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Kind != "image" {
		t.Fatalf("expected image kind, got %q", got.Kind)
	}
	if got.Encoding != "base64" {
		t.Fatalf("expected base64 encoding, got %q", got.Encoding)
	}
	if got.ContentBase == "" {
		t.Fatalf("expected base64 image content")
	}
	if !strings.HasPrefix(got.MIMEType, "image/") {
		t.Fatalf("expected image mime type, got %q", got.MIMEType)
	}
}

func TestWorkspaceFilesHandler_HidesBinaryContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")
	if err := os.WriteFile(filepath.Join(artifactsRoot, "archive.bin"), []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/?path=archive.bin&root="+url.QueryEscape(artifactsRoot), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got workspaceFilePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Kind != "binary" {
		t.Fatalf("expected binary kind, got %q", got.Kind)
	}
	if !got.IsBinary {
		t.Fatalf("expected binary flag to be true")
	}
	if got.Content != "" || got.ContentBase != "" {
		t.Fatalf("expected binary content to be omitted, got content=%q base64=%q", got.Content, got.ContentBase)
	}
}

func TestWorkspaceFilesHandler_ReadsAbsolutePathWithRelativeRoot(t *testing.T) {
	rootParent := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(rootParent); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	root := filepath.Join(rootParent, "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")
	targetPath := filepath.Join(artifactsRoot, "note.md")
	if err := os.WriteFile(targetPath, []byte("# Title\n\nhello"), 0o644); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	req := httptest.NewRequest(
		http.MethodGet,
		"/?path="+url.QueryEscape(targetPath)+"&root="+url.QueryEscape(filepath.ToSlash(filepath.Join("workspace", "artifacts"))),
		nil,
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got workspaceFilePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Kind != "markdown" {
		t.Fatalf("expected markdown kind, got %q", got.Kind)
	}
}

func TestWorkspaceFilesHandler_CreatesDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	body := strings.NewReader(`{"parent_path":".","name":"reports"}`)
	req := httptest.NewRequest(http.MethodPost, "/?root="+url.QueryEscape(artifactsRoot), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%q", rec.Code, rec.Body.String())
	}

	if info, err := os.Stat(filepath.Join(artifactsRoot, "reports")); err != nil {
		t.Fatalf("expected created directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected reports to be a directory")
	}
}

func TestWorkspaceFilesHandler_RenamesDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	artifactsRoot := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(filepath.Join(artifactsRoot, "reports"), 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}

	handler := newWorkspaceFilesHandler(root, zerolog.New(io.Discard))
	body := strings.NewReader(`{"path":"reports","new_name":"weekly"}`)
	req := httptest.NewRequest(http.MethodPatch, "/?root="+url.QueryEscape(artifactsRoot), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(artifactsRoot, "reports")); !os.IsNotExist(err) {
		t.Fatalf("expected source directory to be renamed, stat err=%v", err)
	}
	if info, err := os.Stat(filepath.Join(artifactsRoot, "weekly")); err != nil {
		t.Fatalf("expected renamed directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected weekly to be a directory")
	}
}
