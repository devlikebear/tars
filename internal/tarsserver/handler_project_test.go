package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
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
	handler := newProjectAPIHandler(projectStore, store, mainSess.ID, nil, nil, nil, zerolog.New(io.Discard))

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
	handler := newProjectAPIHandler(projectStore, store, "", nil, nil, nil, zerolog.New(io.Discard))

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
	broker := newProjectDashboardBroker()
	handler := newProjectAPIHandler(projectStore, store, mainSess.ID, nil, nil, broker, zerolog.New(io.Discard))

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

	events, unsubscribe := broker.subscribe()
	defer unsubscribe()

	statePatchReq2 := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+payload.Project.ID+"/state", strings.NewReader(`{
		"phase":"review",
		"status":"paused",
		"next_action":"Wait for feedback"
	}`))
	statePatchReq2.Header.Set("Content-Type", "application/json")
	statePatchRec2 := httptest.NewRecorder()
	handler.ServeHTTP(statePatchRec2, statePatchReq2)
	if statePatchRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for second state patch, got %d body=%q", statePatchRec2.Code, statePatchRec2.Body.String())
	}

	select {
	case evt := <-events:
		if evt.ProjectID != payload.Project.ID || evt.Kind != "activity" {
			t.Fatalf("unexpected dashboard event: %+v", evt)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected dashboard event for state patch")
	}
}

func TestProjectAPI_ActivityRoutes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	projectStore := project.NewStore(root, nil)
	broker := newProjectDashboardBroker()
	handler := newProjectAPIHandler(projectStore, store, "", nil, nil, broker, zerolog.New(io.Discard))

	created, err := projectStore.Create(project.CreateInput{Name: "Ops A", Type: "operations"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	appendReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/activity", strings.NewReader(`{
		"task_id":"task-1",
		"source":"pm",
		"kind":"assignment",
		"status":"queued",
		"message":"Assign task-1 to dev-1",
		"meta":{"agent":"dev-1"}
	}`))
	appendReq.Header.Set("Content-Type", "application/json")
	appendRec := httptest.NewRecorder()
	handler.ServeHTTP(appendRec, appendReq)
	if appendRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activity append, got %d body=%q", appendRec.Code, appendRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/activity", strings.NewReader(`{
		"task_id":"task-1",
		"source":"agent",
		"agent":"dev-1",
		"kind":"task_status",
		"status":"in_progress",
		"message":"Started implementing tests"
	}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for second activity append, got %d body=%q", secondRec.Code, secondRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/"+created.ID+"/activity?limit=1", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activity list, got %d body=%q", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Count int                `json:"count"`
		Items []project.Activity `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode activity payload: %v", err)
	}
	if payload.Count != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected one limited activity item, got %+v", payload)
	}
	if payload.Items[0].Status != "in_progress" {
		t.Fatalf("expected newest activity first, got %+v", payload.Items)
	}

	events, unsubscribe := broker.subscribe()
	defer unsubscribe()

	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/activity", strings.NewReader(`{
		"task_id":"task-2",
		"source":"agent",
		"agent":"dev-2",
		"kind":"task_status",
		"status":"review",
		"message":"Ready for review"
	}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdRec := httptest.NewRecorder()
	handler.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for third activity append, got %d body=%q", thirdRec.Code, thirdRec.Body.String())
	}

	select {
	case evt := <-events:
		if evt.ProjectID != created.ID || evt.Kind != "activity" {
			t.Fatalf("unexpected dashboard event: %+v", evt)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected dashboard activity event")
	}
}

func TestProjectAPI_BoardRoutes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	projectStore := project.NewStore(root, nil)
	broker := newProjectDashboardBroker()
	handler := newProjectAPIHandler(projectStore, store, "", nil, nil, broker, zerolog.New(io.Discard))

	created, err := projectStore.Create(project.CreateInput{Name: "Ops A", Type: "operations"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/projects/"+created.ID+"/board", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for board get, got %d body=%q", getRec.Code, getRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+created.ID+"/board", strings.NewReader(`{
		"tasks":[
			{
				"id":"task-1",
				"title":"Build dashboard",
				"status":"review",
				"assignee":"dev-1",
				"role":"developer",
				"review_required":true,
				"test_command":"go test ./internal/tarsserver",
				"build_command":"go test ./..."
			}
		]
	}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for board patch, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}

	var board project.Board
	if err := json.Unmarshal(patchRec.Body.Bytes(), &board); err != nil {
		t.Fatalf("decode patched board: %v", err)
	}
	if len(board.Tasks) != 1 || board.Tasks[0].Status != "review" {
		t.Fatalf("unexpected patched board: %+v", board)
	}

	events, unsubscribe := broker.subscribe()
	defer unsubscribe()

	secondPatchReq := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+created.ID+"/board", strings.NewReader(`{
		"tasks":[
			{
				"id":"task-1",
				"title":"Build dashboard",
				"status":"done",
				"assignee":"dev-1"
			}
		]
	}`))
	secondPatchReq.Header.Set("Content-Type", "application/json")
	secondPatchRec := httptest.NewRecorder()
	handler.ServeHTTP(secondPatchRec, secondPatchReq)
	if secondPatchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for second board patch, got %d body=%q", secondPatchRec.Code, secondPatchRec.Body.String())
	}

	select {
	case evt := <-events:
		if evt.ProjectID != created.ID || evt.Kind != "board" {
			t.Fatalf("unexpected dashboard event: %+v", evt)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected dashboard board event")
	}
}

func TestProjectAPI_DispatchRoute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	projectStore := project.NewStore(root, nil)
	broker := newProjectDashboardBroker()

	codexExecutor, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name: "codex-cli",
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string) (string, error) {
			return `<task-report>
status: completed
summary: implemented
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/301
branch: feat/task-1
pr: https://github.com/devlikebear/tars/pull/401
notes: ready for review
</task-report>`, nil
		},
	})
	if err != nil {
		t.Fatalf("new codex executor: %v", err)
	}
	claudeExecutor, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name: "claude-code",
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string) (string, error) {
			return `<task-report>
status: approved
summary: reviewed
tests: passed
build: passed
issue: https://github.com/devlikebear/tars/issues/301
branch: feat/task-1
pr: https://github.com/devlikebear/tars/pull/401
notes: approved
</task-report>`, nil
		},
	})
	if err != nil {
		t.Fatalf("new claude executor: %v", err)
	}

	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []gateway.AgentExecutor{codexExecutor, claudeExecutor},
		DefaultAgent: "codex-cli",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	})

	taskRunner := gateway.NewProjectTaskRunner(runtime, "")
	handler := newProjectAPIHandler(projectStore, store, "", taskRunner, func(context.Context) error { return nil }, broker, zerolog.New(io.Discard))

	created, err := projectStore.Create(project.CreateInput{Name: "Dispatch Project", Type: "operations"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := projectStore.UpdateBoard(created.ID, project.BoardUpdateInput{
		Tasks: []project.BoardTask{
			{
				ID:             "task-1",
				Title:          "Build dashboard",
				Status:         "todo",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	events, unsubscribe := broker.subscribe()
	defer unsubscribe()

	todoReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/dispatch", strings.NewReader(`{"stage":"todo"}`))
	todoReq.Header.Set("Content-Type", "application/json")
	todoRec := httptest.NewRecorder()
	handler.ServeHTTP(todoRec, todoReq)
	if todoRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for todo dispatch, got %d body=%q", todoRec.Code, todoRec.Body.String())
	}

	boardAfterTodo, err := projectStore.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board after todo: %v", err)
	}
	if len(boardAfterTodo.Tasks) != 1 || boardAfterTodo.Tasks[0].Status != "review" {
		t.Fatalf("expected task to move to review, got %+v", boardAfterTodo.Tasks)
	}

	reviewReq := httptest.NewRequest(http.MethodPost, "/v1/projects/"+created.ID+"/dispatch", strings.NewReader(`{"stage":"review"}`))
	reviewReq.Header.Set("Content-Type", "application/json")
	reviewRec := httptest.NewRecorder()
	handler.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for review dispatch, got %d body=%q", reviewRec.Code, reviewRec.Body.String())
	}

	boardAfterReview, err := projectStore.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board after review: %v", err)
	}
	if len(boardAfterReview.Tasks) != 1 || boardAfterReview.Tasks[0].Status != "done" {
		t.Fatalf("expected task to move to done, got %+v", boardAfterReview.Tasks)
	}

	select {
	case evt := <-events:
		if evt.ProjectID != created.ID {
			t.Fatalf("unexpected dashboard event: %+v", evt)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected dashboard refresh event after dispatch")
	}
}
