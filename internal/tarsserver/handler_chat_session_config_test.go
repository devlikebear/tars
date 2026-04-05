package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

func TestChatAPIHandler_ChatRequestAppliesSessionConfigToPromptAndTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetToolConfig(sess.ID, &session.SessionToolConfig{
		ToolsEnabled:  []string{"read_file"},
		SkillsEnabled: []string{"notes"},
	}); err != nil {
		t.Fatalf("set tool config: %v", err)
	}

	manager := newSessionConfigTestSkillManager(t, root)
	client := &mockLLMClient{}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		client,
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{Extensions: manager},
	)

	body := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","message":"/project-start 계획 세워줘"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected chat status 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if client.callCount != 1 {
		t.Fatalf("expected one llm call, got %d", client.callCount)
	}

	if len(client.seenTools) == 0 {
		t.Fatal("expected tool schemas to be sent to llm")
	}
	if got := client.seenTools[0]; len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("expected only read_file to be injected, got %+v", got)
	}

	if len(client.seenMessages) == 0 || len(client.seenMessages[0]) == 0 {
		t.Fatal("expected system prompt to be sent to llm")
	}
	systemPrompt := client.seenMessages[0][0].Content
	if strings.Contains(systemPrompt, "<name>project-start</name>") {
		t.Fatalf("expected disabled project-start skill to be excluded from prompt, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "User invoked /project-start.") {
		t.Fatalf("expected disabled project-start explicit routing to be skipped, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "<name>notes</name>") {
		t.Fatalf("expected enabled notes skill to remain in prompt, got %q", systemPrompt)
	}
}

func TestChatAPIHandler_ContextEndpointReflectsSessionConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetToolConfig(sess.ID, &session.SessionToolConfig{
		ToolsEnabled:  []string{"read_file"},
		SkillsEnabled: []string{"notes"},
	}); err != nil {
		t.Fatalf("set tool config: %v", err)
	}

	manager := newSessionConfigTestSkillManager(t, root)
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{Extensions: manager},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected context status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		ToolNames    []string `json:"tool_names"`
		SkillNames   []string `json:"skill_names"`
		SystemPrompt string   `json:"system_prompt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode context payload: %v", err)
	}

	if got := payload.ToolNames; len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("expected only read_file in context preview, got %+v", got)
	}
	if got := strings.Join(payload.SkillNames, ","); got != "notes" {
		t.Fatalf("expected only notes skill in context preview, got %+v", payload.SkillNames)
	}
	if strings.Contains(payload.SystemPrompt, "<name>project-start</name>") {
		t.Fatalf("expected disabled project-start skill to be excluded from preview prompt, got %q", payload.SystemPrompt)
	}
}

func newSessionConfigTestSkillManager(t *testing.T, workspaceDir string) *extensions.Manager {
	t.Helper()
	skillRoot := filepath.Join(workspaceDir, "skills")
	writeSkillFile(t, filepath.Join(skillRoot, "project-start", "SKILL.md"), `---
name: project-start
description: start projects
user-invocable: true
---
# Project Start
`)
	writeSkillFile(t, filepath.Join(skillRoot, "notes", "SKILL.md"), `---
name: notes
description: take notes
user-invocable: true
---
# Notes
`)

	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir:   workspaceDir,
		SkillsEnabled:  true,
		PluginsEnabled: false,
		SkillSources: []skill.SourceDir{
			{Source: skill.SourceWorkspace, Dir: skillRoot},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	return manager
}
