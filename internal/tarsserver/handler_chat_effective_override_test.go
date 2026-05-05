package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/rs/zerolog"
)

func TestChatContext_AppliesPromptOverrideFromSettingsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Drop a settings.json in the active cwd (artifact dir) that injects
	// a prompt_override. The session itself has no prompt_override set, so
	// any override appearing in the system prompt must come from the
	// settings file via the override service.
	cwd := sess.WorkDirs[0]
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .tars: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"),
		[]byte(`{"prompt_override":"OVERRIDE-FROM-SETTINGS"}`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	tooling := chatToolingOptions{
		OverrideService: sessionoverride.NewService(store),
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, &mockLLMClient{}, nil, zerolog.Nop(), 4, nil, "", tooling,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(payload.SystemPrompt, "OVERRIDE-FROM-SETTINGS") {
		t.Fatalf("expected system prompt to include settings-file override, got: %q",
			payload.SystemPrompt)
	}
}

func TestChatContext_NoOverrideService_FallsBackToSessionFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetPromptOverride(sess.ID, "BASE-PROMPT"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}

	// Drop a settings.json that would override — but since OverrideService
	// is nil, the chat handler must ignore it and use the base value.
	cwd := sess.WorkDirs[0]
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"),
		[]byte(`{"prompt_override":"SHOULD-NOT-APPEAR"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, &mockLLMClient{}, nil, zerolog.Nop(), 4, nil, "", chatToolingOptions{},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(payload.SystemPrompt, "BASE-PROMPT") {
		t.Fatalf("expected base prompt to remain when OverrideService is nil, got: %q",
			payload.SystemPrompt)
	}
	if strings.Contains(payload.SystemPrompt, "SHOULD-NOT-APPEAR") {
		t.Fatalf("settings.json override should not leak in when service is nil, got: %q",
			payload.SystemPrompt)
	}
}
