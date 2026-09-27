package tarsserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if client.seenWorkDirs[0] != projectDir {
		t.Fatalf("work dir = %q, want the session's current directory %q", client.seenWorkDirs[0], projectDir)
	}
	if !client.seenPersist[0] {
		t.Fatal("a chat turn must ask the provider to save the upstream session it resumes next turn")
	}
}
