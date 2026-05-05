package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
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
		llm.RoleAgentRuntimePlanner: llm.TierHeavy,
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
			Name  string `json:"name"`
			Group string `json:"group"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	names := map[string]bool{}
	groups := map[string]string{}
	for _, item := range payload.Tools {
		names[item.Name] = true
		groups[item.Name] = item.Group
	}
	for _, want := range []string{
		"read_file",
		"write_file",
		"edit_file",
		"project_skill",
		"workspace",
		"memory",
		"subagents_plan",
		"subagents_run",
		"subagents_orchestrate",
	} {
		if !names[want] {
			t.Fatalf("expected tool %q in /v1/chat/tools, got %+v", want, names)
		}
	}
	if groups["read_file"] != "files" {
		t.Fatalf("expected read_file to be tagged as files, got %+v", groups)
	}
	if groups["project_skill"] != "files" {
		t.Fatalf("expected project_skill to be tagged as files, got %+v", groups)
	}
	if groups["memory"] != "memory" {
		t.Fatalf("expected memory to be tagged as memory, got %+v", groups)
	}
}

func TestChatAPIHandler_ToolsEndpointIncludesMCPServers(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir: root,
		MCPBaseServers: []config.MCPServer{
			{Name: "echo", Command: "node", Args: []string{"server.mjs"}},
		},
	})
	if err != nil {
		t.Fatalf("new extensions manager: %v", err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload extensions manager: %v", err)
	}

	tooling := defaultChatToolingOptions()
	tooling.Extensions = manager
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		session.NewStore(root),
		&mockLLMClient{},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		tooling,
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/tools", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected tools endpoint 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		MCPServers []string `json:"mcp_servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsString(payload.MCPServers, "echo") {
		t.Fatalf("expected echo mcp server in /v1/chat/tools, got %+v", payload.MCPServers)
	}
}
