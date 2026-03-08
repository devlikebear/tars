package tarsserver

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatAPI_ProjectContextFromRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	projectStore := project.NewStore(root, nil)
	created, err := projectStore.Create(project.CreateInput{
		Name:         "Gateway Ops",
		Type:         "operations",
		Objective:    "Keep gateway runtime stable",
		Instructions: "Prefer safe tools and summarize operational impact",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	mockClient := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"run this project task","project_id":"`+created.ID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(mockClient.seenMessages) == 0 || len(mockClient.seenMessages[0]) == 0 {
		t.Fatalf("expected captured llm messages")
	}
	systemPrompt := mockClient.seenMessages[0][0].Content
	if !strings.Contains(systemPrompt, "Active Project") {
		t.Fatalf("expected active project section in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Gateway Ops") {
		t.Fatalf("expected project name in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Keep gateway runtime stable") {
		t.Fatalf("expected project objective in system prompt, got %q", systemPrompt)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatalf("expected created session")
	}
	if sessions[0].ProjectID != created.ID {
		t.Fatalf("expected session project_id %q, got %q", created.ID, sessions[0].ProjectID)
	}
}

func TestChatAPI_RelevantMemoryIsInjectedIntoSystemPrompt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers black coffee.",
		SourceSession: "seed",
		Importance:    9,
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	mockClient := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"what coffee do i prefer?"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(mockClient.seenMessages) == 0 || len(mockClient.seenMessages[0]) == 0 {
		t.Fatalf("expected captured llm messages")
	}
	systemPrompt := mockClient.seenMessages[0][0].Content
	if !strings.Contains(systemPrompt, "## Relevant Memory") {
		t.Fatalf("expected relevant memory section in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "black coffee") {
		t.Fatalf("expected relevant memory content in system prompt, got %q", systemPrompt)
	}
}

func TestChatAPI_DebugLogIncludesContextBudgetStats(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 3, 7, 2, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers black coffee.",
		SourceSession: "seed",
		Importance:    8,
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(sess.ID), session.Message{
		Role:      "user",
		Content:   "hello there",
		Timestamp: time.Date(2026, 3, 7, 2, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append seed history: %v", err)
	}

	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	mockClient := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"session_id":"`+sess.ID+`","message":"what coffee do i prefer?"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	line := logs.String()
	for _, field := range []string{`"history_tokens":`, `"relevant_memory_count":`, `"compaction_used":`, `"system_prompt_tokens":`} {
		if !strings.Contains(line, field) {
			t.Fatalf("expected debug log to include %s, got %q", field, line)
		}
	}
}
