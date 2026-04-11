package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatAPIHandler_ToolsEndpointIncludesWorkspaceEditingBuiltins(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	router, _, err := llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleGatewayPlanner: llm.TierHeavy,
	})
	if err != nil {
		t.Fatalf("new fake router: %v", err)
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		session.NewStore(root),
		&mockLLMClient{},
		router,
		zerolog.Nop(),
		4,
		nil,
		"",
		defaultChatToolingOptions(),
	)
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
		"workspace",
		"memory",
		"knowledge",
		"subagents_plan",
		"subagents_run",
		"subagents_orchestrate",
	} {
		if !names[want] {
			t.Fatalf("expected tool %q in /v1/chat/tools, got %+v", want, names)
		}
	}
}
