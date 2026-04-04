package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatAPIHandler_ToolsEndpointIncludesWorkspaceEditingBuiltins(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	handler := newChatAPIHandler(root, session.NewStore(root), &mockLLMClient{}, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/tools", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected tools endpoint 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	names := map[string]bool{}
	for _, item := range payload.Tools {
		names[item.Name] = true
	}
	for _, want := range []string{
		"read_file",
		"write_file",
		"edit_file",
		"workspace_sysprompt_get",
		"workspace_sysprompt_set",
		"agent_sysprompt_get",
		"agent_sysprompt_set",
	} {
		if !names[want] {
			t.Fatalf("expected tool %q in /v1/chat/tools, got %+v", want, names)
		}
	}
}
