package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/usage"
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
		nil,
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
		nil,
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

func TestChatAPIHandler_PriorContextPreviewEndpointReturnsExactSectionAndItems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("Project atlas launch prefers risk-first release notes.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{MemoryCache: newMemoryCache(time.Minute)},
	)

	body := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","query":"atlas launch release notes"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/prior-context/preview", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected prior context status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		SessionID            string `json:"session_id"`
		Query                string `json:"query"`
		Section              string `json:"section"`
		RelevantTokens       int    `json:"relevant_tokens"`
		RelevantBudgetTokens int    `json:"relevant_budget_tokens"`
		BudgetPercent        int    `json:"budget_percent"`
		Items                []struct {
			Source    string `json:"source"`
			SourceTag string `json:"source_tag"`
			Snippet   string `json:"snippet"`
			Tokens    int    `json:"tokens"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode prior context payload: %v", err)
	}

	if payload.SessionID != sess.ID {
		t.Fatalf("expected session id %q, got %q", sess.ID, payload.SessionID)
	}
	if payload.Query != "atlas launch release notes" {
		t.Fatalf("expected query echo, got %q", payload.Query)
	}
	if !strings.Contains(payload.Section, "## Prior Context") {
		t.Fatalf("expected exact prior context section, got %q", payload.Section)
	}
	if !strings.Contains(payload.Section, "Project atlas launch prefers risk-first release notes.") {
		t.Fatalf("expected section text from memory, got %q", payload.Section)
	}
	if payload.RelevantTokens <= 0 || payload.RelevantBudgetTokens <= 0 || payload.BudgetPercent <= 0 {
		t.Fatalf("expected token budget metadata, got %+v", payload)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one prior context item, got %+v", payload.Items)
	}
	item := payload.Items[0]
	if item.Source != "MEMORY.md" || item.SourceTag != "project" {
		t.Fatalf("expected project source badge for MEMORY.md, got %+v", item)
	}
	if item.Tokens <= 0 || !strings.Contains(item.Snippet, "atlas launch") {
		t.Fatalf("expected item token estimate and snippet, got %+v", item)
	}
}

func TestChatAPIHandler_PriorContextPreviewEmptyQueryReturnsRecentFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("Recent project note one.\nRecent project note two.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{MemoryCache: newMemoryCache(time.Minute)},
	)

	body := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","query":""}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/prior-context/preview", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Mode                string `json:"mode"`
		Section             string `json:"section"`
		Items               []any  `json:"items"`
		RecentFallbackItems []struct {
			Source    string `json:"source"`
			SourceTag string `json:"source_tag"`
			Snippet   string `json:"snippet"`
			Tokens    int    `json:"tokens"`
		} `json:"recent_fallback_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Mode != "recent" {
		t.Fatalf("expected mode=recent for empty query, got %q", payload.Mode)
	}
	if payload.Section != "" {
		t.Fatalf("expected empty Section in recent mode, got %q", payload.Section)
	}
	if len(payload.RecentFallbackItems) == 0 {
		t.Fatalf("expected at least one recent fallback item, got none")
	}
	found := false
	for _, item := range payload.RecentFallbackItems {
		if item.Source == "MEMORY.md" && item.SourceTag == "project" && strings.Contains(item.Snippet, "project note") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MEMORY.md fallback item with project tag, got %+v", payload.RecentFallbackItems)
	}
}

func TestChatAPIHandler_PriorContextPreviewIncludesBelowThresholdCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	// Single-keyword match against MEMORY.md scores below 100 because the
	// keyword scorer awards 25 per term + base 100 only when the line matches
	// AND the file is recent enough; we use an old-content line that lacks the
	// query terms so it cannot pass the threshold.
	memoryContent := "" +
		"alpha solo line that mentions zephyr only.\n" +
		"unrelated control line about giraffes.\n"
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(memoryContent), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{MemoryCache: newMemoryCache(time.Minute)},
	)

	body := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","query":"zephyr"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/prior-context/preview", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Mode                string `json:"mode"`
		Items               []any  `json:"items"`
		BelowThresholdItems []struct {
			Source  string `json:"source"`
			Snippet string `json:"snippet"`
		} `json:"below_threshold_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Mode != "default" {
		t.Fatalf("expected mode=default for non-empty query, got %q", payload.Mode)
	}
	// We don't strictly require Items to be empty (depends on recency boost),
	// but the response payload must always include the below_threshold field
	// (never null) so the frontend can render unconditionally.
	if payload.BelowThresholdItems == nil {
		t.Fatalf("expected below_threshold_items field present, got nil")
	}
}

func TestChatAPIHandler_ContextEndpointReflectsSessionGroupConfig(t *testing.T) {
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
		ToolsAllowGroups: []string{"files", "web"},
		ToolsDenyGroups:  []string{"shell"},
	}); err != nil {
		t.Fatalf("set tool config: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		defaultChatToolingOptions(),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected context status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		ToolNames []string `json:"tool_names"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode context payload: %v", err)
	}
	for _, expected := range []string{"read_file", "list_dir", "glob"} {
		if !containsString(payload.ToolNames, expected) {
			t.Fatalf("expected %s in context preview, got %+v", expected, payload.ToolNames)
		}
	}
	for _, denied := range []string{"exec", "process", "memory", "session"} {
		if containsString(payload.ToolNames, denied) {
			t.Fatalf("expected %s to be excluded from context preview, got %+v", denied, payload.ToolNames)
		}
	}
}

func TestChatAPIHandler_ContextEndpointReflectsSessionGroupConfigAfterPatchAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		defaultChatToolingOptions(),
	)
	sessionHandler := newSessionAPIHandler(store, zerolog.Nop())

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/sessions/"+sess.ID+"/config", strings.NewReader(`{"tools_allow_groups":["files","web"],"tools_deny_groups":["shell"],"tools_custom":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	patchReq.RemoteAddr = "127.0.0.1:12345"
	patchRec := httptest.NewRecorder()
	sessionHandler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected context status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		ToolNames []string `json:"tool_names"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode context payload: %v", err)
	}
	for _, expected := range []string{"read_file", "list_dir", "glob"} {
		if !containsString(payload.ToolNames, expected) {
			t.Fatalf("expected %s in context preview after patch API, got %+v", expected, payload.ToolNames)
		}
	}
}

func TestSessionAPIHandler_ConfigPatchRecordsUsageSignal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new usage tracker: %v", err)
	}
	handler := newSessionAPIHandlerWithUsage(store, zerolog.Nop(), tracker)

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/sessions/"+sess.ID+"/config", strings.NewReader(`{
		"tools_custom": true,
		"tools_enabled": ["read_file", "list_dir"],
		"tools_disabled": ["exec"],
		"tools_allow_groups": ["files"],
		"skills_custom": true,
		"skills_enabled": ["notes"],
		"mcp_enabled": ["local-fs"]
	}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	patchReq.RemoteAddr = "127.0.0.1:12345"
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}

	summary, err := tracker.Signals("today")
	if err != nil {
		t.Fatalf("signals: %v", err)
	}
	if len(summary.Rows) != 1 {
		t.Fatalf("expected one signal row, got %+v", summary.Rows)
	}
	row := summary.Rows[0]
	if row.Name != "session.tool_config.updated" || row.Source != "api" || row.Count != 1 {
		t.Fatalf("unexpected signal row: %+v", row)
	}
	for key, want := range map[string]string{
		"tools_custom":             "true",
		"skills_custom":            "true",
		"tools_enabled_count":      "2",
		"tools_disabled_count":     "1",
		"tools_allow_groups_count": "1",
		"skills_enabled_count":     "1",
		"mcp_enabled_count":        "1",
	} {
		if got := row.Dimensions[key]; got != want {
			t.Fatalf("expected signal dimension %s=%q, got %q in %+v", key, want, got, row.Dimensions)
		}
	}
}

func TestChatAPIHandler_ContextEndpointIncludesLastCompactionMode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetLastCompactionMode(sess.ID, "deterministic"); err != nil {
		t.Fatalf("set last compaction mode: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		defaultChatToolingOptions(),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected context status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		CompactionLastMode string `json:"compaction_last_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode context payload: %v", err)
	}
	if payload.CompactionLastMode != "deterministic" {
		t.Fatalf("expected deterministic compaction mode, got %q", payload.CompactionLastMode)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
