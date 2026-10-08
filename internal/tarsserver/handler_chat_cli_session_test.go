package tarsserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// A chat turn tells a CLI provider to work in the session's current
// directory and to save the upstream session it starts, because the next
// turn resumes it.
func TestChatAPI_PassesSessionDirectoryAndPersistsUpstreamSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("cli session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	// The store saves the directory with symlinks resolved, so on macOS,
	// where t.TempDir() is under /var -> /private/var, it differs from
	// projectDir.
	saved, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	currentDir := saved.CurrentDir
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, client, zerolog.New(io.Discard))

	raw, err := json.Marshal(map[string]any{"session_id": sess.ID, "message": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(client.seenWorkDirs) == 0 {
		t.Fatal("expected an LLM call")
	}
	if client.seenWorkDirs[0] != currentDir {
		t.Fatalf("work dir = %q, want the session's current directory %q", client.seenWorkDirs[0], currentDir)
	}
	if !client.seenPersist[0] {
		t.Fatal("a chat turn must ask the provider to save the upstream session it resumes next turn")
	}
}

// A session given more than one folder (as a focus pipeline's extra_dirs
// does) reaches a CLI-backed provider through ChatOptions.AddDirs: every
// registered work dir besides the active cwd.
func TestChatAPI_PassesExtraWorkDirsAsAddDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	primaryDir := filepath.Join(root, "primary")
	extraDir := filepath.Join(root, "extra")
	for _, dir := range []string{primaryDir, extraDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	store := session.NewStore(root)
	sess, err := store.Create("multi-folder session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{primaryDir, extraDir}, primaryDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	saved, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	currentDir := saved.CurrentDir
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, client, zerolog.New(io.Discard))

	raw, err := json.Marshal(map[string]any{"session_id": sess.ID, "message": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(client.seenAddDirs) == 0 {
		t.Fatal("expected an LLM call")
	}
	// The store also always registers the session's own artifact dir as a
	// work dir, so assert the requested extra folder is present and the
	// active cwd (passed separately as WorkDir) is not repeated here,
	// rather than asserting an exact list.
	addDirs := client.seenAddDirs[0]
	foundExtra := false
	for _, dir := range addDirs {
		if dir == currentDir {
			t.Fatalf("add dirs = %v must not repeat the active cwd %q", addDirs, currentDir)
		}
		if strings.HasSuffix(dir, string(filepath.Separator)+"extra") {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Fatalf("add dirs = %v missing the extra folder", addDirs)
	}
}
