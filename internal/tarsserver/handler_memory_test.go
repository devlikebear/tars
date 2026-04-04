package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

