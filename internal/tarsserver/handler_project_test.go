package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
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

func TestProjectAPI_PatchUpdatesPolicyFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	projectStore := project.NewStore(root, nil)
	handler := newProjectAPIHandler(projectStore, store, "", zerolog.New(io.Discard))

	created, err := projectStore.Create(project.CreateInput{Name: "Ops A", Type: "operations"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+created.ID, strings.NewReader(`{
		"objective":"Keep service green",
		"instructions":"Check alerts first",
		"tools_allow":["read_file","exec"],
		"tools_risk_max":"medium",
		"skills_allow":["deploy"]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for patch, got %d body=%q", rec.Code, rec.Body.String())
	}
	var updated project.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated project: %v", err)
	}
	if updated.Objective != "Keep service green" {
		t.Fatalf("expected updated objective, got %q", updated.Objective)
	}
	if !strings.Contains(updated.Body, "Check alerts first") {
		t.Fatalf("expected updated instructions, got %q", updated.Body)
	}
	if got := strings.Join(updated.ToolsAllow, ","); got != "read_file,exec" {
		t.Fatalf("unexpected tools_allow: %q", got)
	}
	if updated.ToolsRiskMax != "medium" {
		t.Fatalf("expected tools_risk_max=medium, got %q", updated.ToolsRiskMax)
	}
	if got := strings.Join(updated.SkillsAllow, ","); got != "deploy" {
		t.Fatalf("unexpected skills_allow: %q", got)
	}
}

func TestProjectAPI_BriefFinalizeAndStateRoutes(t *testing.T) {
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

	briefReq := httptest.NewRequest(http.MethodPatch, "/v1/project-briefs/"+mainSess.ID, strings.NewReader(`{
		"title":"Orbit Hearts",
		"goal":"Write a serialized space opera",
		"kind":"serial",
		"premise":"Two rival navigators chase a dead-star map.",
		"open_questions":["Who betrays the crew in arc one?"],
		"status":"ready"
	}`))
	briefReq.Header.Set("Content-Type", "application/json")
	briefRec := httptest.NewRecorder()
	handler.ServeHTTP(briefRec, briefReq)
	if briefRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for brief patch, got %d body=%q", briefRec.Code, briefRec.Body.String())
	}

	finalizeReq := httptest.NewRequest(http.MethodPost, "/v1/project-briefs/"+mainSess.ID+"/finalize", nil)
	finalizeRec := httptest.NewRecorder()
	handler.ServeHTTP(finalizeRec, finalizeReq)
	if finalizeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for brief finalize, got %d body=%q", finalizeRec.Code, finalizeRec.Body.String())
	}
	var payload struct {
		Project project.Project `json:"project"`
	}
	if err := json.Unmarshal(finalizeRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode finalize payload: %v", err)
	}
	if strings.TrimSpace(payload.Project.ID) == "" {
		t.Fatalf("expected finalized project id")
	}

	stateGetReq := httptest.NewRequest(http.MethodGet, "/v1/projects/"+payload.Project.ID+"/state", nil)
	stateGetRec := httptest.NewRecorder()
	handler.ServeHTTP(stateGetRec, stateGetReq)
	if stateGetRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for state get, got %d body=%q", stateGetRec.Code, stateGetRec.Body.String())
	}

	statePatchReq := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+payload.Project.ID+"/state", strings.NewReader(`{
		"phase":"executing",
		"status":"active",
		"next_action":"Draft chapter one",
		"remaining_tasks":["outline act one","draft chapter one"]
	}`))
	statePatchReq.Header.Set("Content-Type", "application/json")
	statePatchRec := httptest.NewRecorder()
	handler.ServeHTTP(statePatchRec, statePatchReq)
	if statePatchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for state patch, got %d body=%q", statePatchRec.Code, statePatchRec.Body.String())
	}
	if !strings.Contains(statePatchRec.Body.String(), "Draft chapter one") {
		t.Fatalf("expected next_action in state patch response, got %q", statePatchRec.Body.String())
	}
}
