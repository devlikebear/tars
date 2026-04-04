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
	"github.com/rs/zerolog"
)

func TestMemoryAPIHandler_KnowledgeCRUDAndGraph(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	handler := newMemoryAPIHandler(root, nil, zerolog.Nop())

	createReq := httptest.NewRequest(http.MethodPost, "/v1/memory/kb/notes", strings.NewReader(`{
		"title":"Coffee Preference",
		"kind":"preference",
		"summary":"User prefers black coffee.",
		"body":"Keep coffee suggestions unsweetened.",
		"tags":["coffee"]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d body=%q", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/memory/kb/notes?query=coffee", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "Coffee Preference") {
		t.Fatalf("unexpected list response: code=%d body=%q", listRec.Code, listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/memory/kb/notes/coffee-preference", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), "unsweetened") {
		t.Fatalf("unexpected get response: code=%d body=%q", getRec.Code, getRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/memory/kb/notes/coffee-preference", strings.NewReader(`{
		"summary":"User strongly prefers black coffee.",
		"links":[{"target":"morning-routine","relation":"supports"}]
	}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK || !strings.Contains(patchRec.Body.String(), "strongly prefers") {
		t.Fatalf("unexpected patch response: code=%d body=%q", patchRec.Code, patchRec.Body.String())
	}

	graphReq := httptest.NewRequest(http.MethodGet, "/v1/memory/kb/graph", nil)
	graphRec := httptest.NewRecorder()
	handler.ServeHTTP(graphRec, graphReq)
	if graphRec.Code != http.StatusOK {
		t.Fatalf("unexpected graph response: code=%d body=%q", graphRec.Code, graphRec.Body.String())
	}
	var graph struct {
		Nodes []memory.KnowledgeGraphNode `json:"nodes"`
		Edges []memory.KnowledgeGraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(graphRec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(graph.Nodes) != 1 || len(graph.Edges) != 1 {
		t.Fatalf("unexpected graph payload: %+v", graph)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/memory/kb/notes/coffee-preference", nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK || !strings.Contains(deleteRec.Body.String(), `"deleted":true`) {
		t.Fatalf("unexpected delete response: code=%d body=%q", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestMemoryAPIHandler_EmptyKnowledgeGraph(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	handler := newMemoryAPIHandler(root, nil, zerolog.Nop())

	graphReq := httptest.NewRequest(http.MethodGet, "/v1/memory/kb/graph", nil)
	graphRec := httptest.NewRecorder()
	handler.ServeHTTP(graphRec, graphReq)
	if graphRec.Code != http.StatusOK {
		t.Fatalf("unexpected graph response: code=%d body=%q", graphRec.Code, graphRec.Body.String())
	}

	var graph struct {
		Nodes []memory.KnowledgeGraphNode `json:"nodes"`
		Edges []memory.KnowledgeGraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(graphRec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("expected empty graph payload, got %+v", graph)
	}
}

func TestMemoryAPIHandler_ListsEditsFilesAndRunsSearch(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("# MEMORY.md\n\n- Existing durable memory\n"), 0o644); err != nil {
		t.Fatalf("write memory file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "experiences.jsonl"), []byte("{\"summary\":\"나는 삼성전자 주식을 보유하고 있어\",\"category\":\"fact\"}\n"), 0o644); err != nil {
		t.Fatalf("write experiences file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "2026-04-04.md"), []byte("daily durable note about stocks\n"), 0o644); err != nil {
		t.Fatalf("write daily file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "index", "entries.jsonl"), []byte("{\"id\":\"entry-1\"}\n"), 0o644); err != nil {
		t.Fatalf("write semantic entries: %v", err)
	}

	handler := newMemoryAPIHandler(root, nil, zerolog.Nop())

	listReq := httptest.NewRequest(http.MethodGet, "/v1/memory/assets", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected assets 200, got %d body=%q", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"path":"MEMORY.md"`) ||
		!strings.Contains(listRec.Body.String(), `"path":"memory/experiences.jsonl"`) ||
		!strings.Contains(listRec.Body.String(), `"path":"memory/2026-04-04.md"`) ||
		!strings.Contains(listRec.Body.String(), `"path":"memory/index/entries.jsonl"`) {
		t.Fatalf("expected durable memory assets in list, got %q", listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), `"path":"memory/wiki/`) {
		t.Fatalf("did not expect KB files in memory asset list, got %q", listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/memory/file?path=MEMORY.md", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), "Existing durable memory") {
		t.Fatalf("unexpected memory file response: code=%d body=%q", getRec.Code, getRec.Body.String())
	}

	saveReq := httptest.NewRequest(http.MethodPut, "/v1/memory/file", strings.NewReader(`{
		"path":"MEMORY.md",
		"content":"# MEMORY.md\n\n- Updated durable memory\n"
	}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	handler.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("unexpected save response: code=%d body=%q", saveRec.Code, saveRec.Body.String())
	}
	updatedRaw, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read updated memory file: %v", err)
	}
	if !strings.Contains(string(updatedRaw), "Updated durable memory") {
		t.Fatalf("expected updated file content, got %q", string(updatedRaw))
	}

	searchReq := httptest.NewRequest(http.MethodPost, "/v1/memory/search", strings.NewReader(`{
		"query":"삼성전자 주식",
		"include_memory":false,
		"include_daily":false
	}`))
	searchReq.Header.Set("Content-Type", "application/json")
	searchRec := httptest.NewRecorder()
	handler.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK || !strings.Contains(searchRec.Body.String(), "삼성전자 주식을 보유하고 있어") {
		t.Fatalf("unexpected memory search response: code=%d body=%q", searchRec.Code, searchRec.Body.String())
	}
}

func TestMemoryAPIHandler_ListsAndEditsSyspromptFiles(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "USER.md")); err != nil {
		t.Fatalf("remove USER.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n\n- follow repo rules\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	handler := newMemoryAPIHandler(root, nil, zerolog.Nop())

	listReq := httptest.NewRequest(http.MethodGet, "/v1/workspace/sysprompt/files", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected sysprompt files 200, got %d body=%q", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"path":"USER.md"`) ||
		!strings.Contains(listRec.Body.String(), `"exists":false`) ||
		!strings.Contains(listRec.Body.String(), `"path":"AGENTS.md"`) ||
		!strings.Contains(listRec.Body.String(), `"prompt_targets":["sub_agent"]`) ||
		!strings.Contains(listRec.Body.String(), `"path":"IDENTITY.md"`) ||
		!strings.Contains(listRec.Body.String(), `"prompt_targets":["main_agent"]`) {
		t.Fatalf("unexpected workspace file list: %q", listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/workspace/sysprompt/file?scope=workspace&path=USER.md", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected sysprompt file 200, got %d body=%q", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"exists":false`) || !strings.Contains(getRec.Body.String(), `# USER.md`) {
		t.Fatalf("expected missing USER.md to include starter content, got %q", getRec.Body.String())
	}

	saveReq := httptest.NewRequest(http.MethodPut, "/v1/workspace/sysprompt/file", strings.NewReader(`{
		"scope":"workspace",
		"path":"USER.md",
		"content":"# USER.md\n\n- prefers concise Korean answers\n"
	}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	handler.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected sysprompt save 200, got %d body=%q", saveRec.Code, saveRec.Body.String())
	}

	updatedRaw, err := os.ReadFile(filepath.Join(root, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if !strings.Contains(string(updatedRaw), "prefers concise Korean answers") {
		t.Fatalf("expected USER.md to be created, got %q", string(updatedRaw))
	}
}
