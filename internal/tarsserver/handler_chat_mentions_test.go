package tarsserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type chatFileMentionCandidatesResponse struct {
	Candidates []chatFileMentionCandidate `json:"candidates"`
}

func TestChatFileMentionCandidatesUseSessionCurrentDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".hidden.md"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("write hidden: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("mention candidates")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	handler := newChatAPIHandler(root, store, &mockLLMClient{}, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/mentions/files?session_id="+url.QueryEscape(sess.ID)+"&q=read", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got chatFileMentionCandidatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Candidates) != 1 {
		t.Fatalf("expected one visible README candidate, got %+v", got.Candidates)
	}
	candidate := got.Candidates[0]
	if candidate.Kind != "file" || candidate.Path != "README.md" || canonicalWorkspacePath(candidate.Root) != canonicalWorkspacePath(projectDir) {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
	if candidate.Token != "@README.md" {
		t.Fatalf("expected insert token @README.md, got %q", candidate.Token)
	}
}

func TestChatFileMentionCandidatesRejectTraversal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("mention traversal")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newChatAPIHandler(root, store, &mockLLMClient{}, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/mentions/files?session_id="+url.QueryEscape(sess.ID)+"&q=../", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for traversal query, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestChatAPIInjectsFileAndDirectoryMentions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	docsDir := filepath.Join(projectDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("# Project\nImportant details."), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "notes.md"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("mention injection")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	client := &mockLLMClient{
		response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}},
	}
	handler := newChatAPIHandler(root, store, client, zerolog.New(io.Discard))

	body := map[string]any{
		"session_id": sess.ID,
		"message":    "summarize @README.md and @docs/",
		"mentions": []chatFileMentionRequest{
			{Kind: "file", Root: projectDir, Path: "README.md"},
			{Kind: "directory", Root: projectDir, Path: "docs"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(client.seenMessages) == 0 {
		t.Fatalf("expected LLM call")
	}
	last := client.seenMessages[0][len(client.seenMessages[0])-1]
	joined := contentBlockText(last.ContentBlocks)
	for _, want := range []string{
		"--- Mentioned file: README.md ---",
		"# Project",
		"--- Mentioned directory: docs ---",
		"notes.md",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected mentioned context to contain %q, got %q", want, joined)
		}
	}
}

func TestChatAPIRejectsMentionOutsideSessionWorkDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	outsideDir := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("mention outside")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	client := &mockLLMClient{
		response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "should not run"}},
	}
	handler := newChatAPIHandler(root, store, client, zerolog.New(io.Discard))

	body := map[string]any{
		"session_id": sess.ID,
		"message":    "read @secret.txt",
		"mentions": []chatFileMentionRequest{
			{Kind: "file", Root: outsideDir, Path: "secret.txt"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if client.callCount != 0 {
		t.Fatalf("expected invalid mention to stop before LLM call, got %d calls", client.callCount)
	}
}

func contentBlockText(blocks []llm.ContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		b.WriteString(block.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
