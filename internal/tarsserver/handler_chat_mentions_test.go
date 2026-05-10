package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
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

func TestChatFileMentionCandidatesShowTarsDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	tarsDir := filepath.Join(projectDir, ".tars", "skills")
	if err := os.MkdirAll(tarsDir, 0o755); err != nil {
		t.Fatalf("mkdir .tars/skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tarsDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("tars dir mention test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	handler := newChatAPIHandler(root, store, &mockLLMClient{}, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/mentions/files?session_id="+url.QueryEscape(sess.ID)+"&q=.tars", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var got chatFileMentionCandidatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Candidates) == 0 {
		t.Fatal("expected .tars directory to appear in mention candidates, got none")
	}
	found := false
	for _, c := range got.Candidates {
		if c.Path == ".tars" && c.Kind == "directory" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf(".tars directory not found in candidates: %+v", got.Candidates)
	}
}

func TestChatFileMentionCandidatesRecursiveSearch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "project")
	deepDir := filepath.Join(projectDir, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deepDir, "deep.md"), []byte("deep"), 0o644); err != nil {
		t.Fatalf("write deep.md: %v", err)
	}
	// node_modules should not be walked
	nodeDir := filepath.Join(projectDir, "node_modules")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nodeDir, "deep.md"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("write node_modules deep.md: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("recursive mention test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	handler := newChatAPIHandler(root, store, &mockLLMClient{}, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/mentions/files?session_id="+url.QueryEscape(sess.ID)+"&q=deep", nil)
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
		t.Fatalf("expected exactly one candidate (deep.md), got %+v", got.Candidates)
	}
	c := got.Candidates[0]
	if c.Name != "deep.md" || c.Path != "a/b/c/deep.md" {
		t.Fatalf("unexpected candidate: %+v", c)
	}
	if c.Token != "@a/b/c/deep.md" {
		t.Fatalf("unexpected token: %q", c.Token)
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

func TestChatAPIInjectsSubagentMentionHints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("subagent mention")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	runtime := newChatMentionTestRuntime(t, root, store)
	client := &mockLLMClient{
		response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}},
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		client,
		nil,
		logger,
		8,
		nil,
		"",
		chatToolingOptions{AgentRuntime: runtime},
	)

	body := map[string]any{
		"session_id": sess.ID,
		"message":    "ask @researcher to inspect the auth flow",
		"subagent_mentions": []chatSubagentMentionRequest{
			{Name: "researcher", Token: "@researcher"},
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
	if len(client.seenMessages) == 0 || len(client.seenMessages[0]) == 0 {
		t.Fatalf("expected LLM messages")
	}
	systemPrompt := client.seenMessages[0][0].Content
	for _, want := range []string{
		"## Mentioned Subagents",
		"researcher",
		"Deep research subagent",
		"subagents_run",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected system prompt to contain %q, got %q", want, systemPrompt)
		}
	}
	responseBody := rec.Body.String()
	for _, want := range []string{
		`"mentioned_subagent_count":1`,
		`"mentioned_subagents":["researcher"]`,
	} {
		if !strings.Contains(responseBody, want) {
			t.Fatalf("expected SSE context info to contain %q, got %q", want, responseBody)
		}
	}
}

func TestChatAPIRejectsUnknownSubagentMention(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("missing subagent mention")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	runtime := newChatMentionTestRuntime(t, root, store)
	client := &mockLLMClient{
		response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "should not run"}},
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		client,
		nil,
		logger,
		8,
		nil,
		"",
		chatToolingOptions{AgentRuntime: runtime},
	)

	body := map[string]any{
		"session_id": sess.ID,
		"message":    "ask @missing to inspect the auth flow",
		"subagent_mentions": []chatSubagentMentionRequest{
			{Name: "missing", Token: "@missing"},
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
	if !strings.Contains(rec.Body.String(), "subagent mention not found") {
		t.Fatalf("expected missing subagent error, got %q", rec.Body.String())
	}
	if client.callCount != 0 {
		t.Fatalf("expected invalid mention to stop before LLM call, got %d calls", client.callCount)
	}
}

func newChatMentionTestRuntime(t *testing.T, root string, store *session.Store) *agentruntime.Runtime {
	t.Helper()
	runPrompt := func(_ context.Context, _ string, prompt string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		return "summary: " + prompt, nil
	}
	explorer, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:        "explorer",
		Description: "Read-only explorer",
		RunPrompt:   runPrompt,
	})
	if err != nil {
		t.Fatalf("new explorer executor: %v", err)
	}
	researcher, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:        "researcher",
		Description: "Deep research subagent",
		Tier:        "heavy",
		RunPrompt:   runPrompt,
	})
	if err != nil {
		t.Fatalf("new researcher executor: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: root,
		SessionStore: store,
		Executors:    []agentruntime.AgentExecutor{explorer, researcher},
		DefaultAgent: "explorer",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close agent runtime: %v", err)
		}
	})
	return runtime
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
