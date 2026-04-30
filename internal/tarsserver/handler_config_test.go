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

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestConfigAPI_SchemaReflectsPatchedValues(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
runtime:
  workspace_dir: ./workspace
llm:
  providers:
    codex:
      kind: openai-codex
      auth_mode: oauth
  tiers:
    heavy:
      provider: codex
      model: gpt-5.5
    standard:
      provider: codex
      model: gpt-5.4
    light:
      provider: codex
      model: gpt-5.4-mini
  default_tier: standard
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	workspaceDir := filepath.Join(dir, "workspace")
	h := newConfigAPIHandler(configPath, cfg, workspaceDir, zerolog.Nop())
	patchBody, _ := json.Marshal(map[string]any{
		"updates": map[string]any{
			"llm_tiers": map[string]any{
				"heavy":    map[string]any{"provider": "codex", "model": "gpt-5.5"},
				"standard": map[string]any{"provider": "codex", "model": "gpt-5.4"},
				"light":    map[string]any{"provider": "codex", "model": "gpt-5.4-mini"},
				"turbo":    map[string]any{"provider": "codex", "model": "gpt-5.4"},
			},
		},
	})
	patchRec := httptest.NewRecorder()
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/config/values", bytes.NewReader(patchBody))
	h.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch 200, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	schemaRec := httptest.NewRecorder()
	schemaReq := httptest.NewRequest(http.MethodGet, "/v1/admin/config/schema", nil)
	h.ServeHTTP(schemaRec, schemaReq)
	if schemaRec.Code != http.StatusOK {
		t.Fatalf("expected schema 200, got %d body=%s", schemaRec.Code, schemaRec.Body.String())
	}
	var payload configSchemaResponse
	if err := json.Unmarshal(schemaRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	tiers, ok := payload.Values["llm_tiers"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_tiers map, got %#v", payload.Values["llm_tiers"])
	}
	if _, ok := tiers["turbo"]; !ok {
		t.Fatalf("expected patched turbo tier in schema values, got %#v", tiers)
	}
	if got := payload.Values["workspace_dir"]; got != workspaceDir {
		t.Fatalf("expected runtime workspace override %q, got %#v", workspaceDir, got)
	}
	if payload.UpdatedAt == "" {
		t.Fatalf("expected schema response to include config file updated_at")
	}
}

func TestConfigAPI_ResetWorkspaceReportsPartialFailures(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission-based removal failure is not reliable as root")
	}
	dir := t.TempDir()
	workspaceDir := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDir, "config"), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "sessions.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	if err := os.Chmod(workspaceDir, 0o500); err != nil {
		t.Fatalf("chmod workspace: %v", err)
	}
	defer func() { _ = os.Chmod(workspaceDir, 0o700) }()

	h := newConfigAPIHandler("", config.Config{}, workspaceDir, zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/reset/workspace", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Error       string                `json:"error"`
		FailedItems []workspaceResetError `json:"failed_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error == "" || len(payload.FailedItems) == 0 {
		t.Fatalf("expected reset failure details, payload=%+v", payload)
	}
}
