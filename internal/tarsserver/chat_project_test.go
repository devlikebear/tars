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

	"github.com/devlikebear/tars/internal/agent"
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

func TestChatAPI_ProjectKickoffWithoutSessionID_CreatesFreshSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockClient := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	handler := newChatAPIHandler(root, store, mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"todo 앱 만드는 프로젝트 시작해줘"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	mainMessages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(mainMessages) != 0 {
		t.Fatalf("expected main transcript to stay untouched, got %+v", mainMessages)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected main + fresh kickoff session, got %+v", sessions)
	}

	var kickoffSession session.Session
	for _, item := range sessions {
		if item.ID == mainSession.ID {
			continue
		}
		kickoffSession = item
	}
	if strings.TrimSpace(kickoffSession.ID) == "" {
		t.Fatalf("expected kickoff session to be created, got %+v", sessions)
	}
	if strings.Contains(rec.Body.String(), mainSession.ID) {
		t.Fatalf("expected response stream to avoid main session id, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), kickoffSession.ID) {
		t.Fatalf("expected response stream to include kickoff session id %q, got %q", kickoffSession.ID, rec.Body.String())
	}

	kickoffMessages, err := session.ReadMessages(store.TranscriptPath(kickoffSession.ID))
	if err != nil {
		t.Fatalf("read kickoff transcript: %v", err)
	}
	if len(kickoffMessages) == 0 || kickoffMessages[0].Content != "todo 앱 만드는 프로젝트 시작해줘" {
		t.Fatalf("expected kickoff transcript to capture user message, got %+v", kickoffMessages)
	}
}

func TestChatAPI_EmitsSkillSelectedStatusForActiveBrief(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	workspaceDir := filepath.Join(root, "workspace")
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(workspaceDir)
	manager := newTestSkillManager(t, root, workspaceDir)
	mockClient := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	status := "collecting"
	goal := "Ship a todo app"
	projectStore := project.NewStore(workspaceDir, nil)
	if _, err := projectStore.UpdateBrief(sess.ID, project.BriefUpdateInput{
		Goal:   &goal,
		Status: &status,
	}); err != nil {
		t.Fatalf("update brief: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		mockClient,
		logger,
		agent.DefaultMaxLoopIters,
		nil,
		"",
		chatToolingOptions{Extensions: manager},
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"session_id":"`+sess.ID+`","message":"로그인은 이메일 기반이면 돼"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"phase":"skill_selected"`) {
		t.Fatalf("expected skill_selected status event, got %q", body)
	}
	if !strings.Contains(body, `"skill_name":"project-start"`) {
		t.Fatalf("expected project-start skill in status event, got %q", body)
	}
	if !strings.Contains(body, `"skill_reason":"active_brief"`) {
		t.Fatalf("expected active_brief reason in status event, got %q", body)
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
