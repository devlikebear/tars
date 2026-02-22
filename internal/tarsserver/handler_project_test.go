package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func TestProjectAPI_CRUDAndActivate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mainSess, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}

	projectStore := project.NewStore(root, nil)
	handler := newProjectAPIHandler(projectStore, store, mainSess.ID, zerolog.New(io.Discard))

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{
		"name":"Ops A",
		"type":"operations",
		"objective":"Operate service A",
		"instructions":"Check alerts first"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for create, got %d body=%q", createRec.Code, createRec.Body.String())
	}
	var created project.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatalf("expected project id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/projects/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for get, got %d body=%q", getRec.Code, getRec.Body.String())
	}

	activateReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/activate", strings.NewReader(`{"session_id":"`+mainSess.ID+`"}`))
	activateReq.Header.Set("Content-Type", "application/json")
	activateRec := httptest.NewRecorder()
	handler.ServeHTTP(activateRec, activateReq)
	if activateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activate, got %d body=%q", activateRec.Code, activateRec.Body.String())
	}

	mainAfter, err := store.Get(mainSess.ID)
	if err != nil {
		t.Fatalf("get main session after activate: %v", err)
	}
	if mainAfter.ProjectID != created.ID {
		t.Fatalf("expected session project_id %q, got %q", created.ID, mainAfter.ProjectID)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/projects/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for delete, got %d body=%q", deleteRec.Code, deleteRec.Body.String())
	}

	archived, err := projectStore.Get(created.ID)
	if err != nil {
		t.Fatalf("get archived project: %v", err)
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %q", archived.Status)
	}
}
