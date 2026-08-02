package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestWorkLedgerAPIListsFilteredWorkspaceWorks(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	running := createWorkLedgerHandlerTestWork(t, store, "workspace-a", "running", workstore.WorkStateRunning)
	createWorkLedgerHandlerTestWork(t, store, "workspace-a", "done", workstore.WorkStateDone)
	createWorkLedgerHandlerTestWork(t, store, "workspace-b", "other", workstore.WorkStateRunning)
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/work/works?state=running&limit=10&offset=0", nil)
	req = req.WithContext(serverauth.WithWorkspaceID(req.Context(), "workspace-a"))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Works  []workstore.Work `json:"works"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(body.Works) != 1 || body.Works[0].ID != running.ID || body.Works[0].WorkspaceID != "workspace-a" {
		t.Fatalf("workspace-filtered works = %#v", body.Works)
	}
	if body.Limit != 10 || body.Offset != 0 {
		t.Fatalf("pagination = %d/%d, want 10/0", body.Limit, body.Offset)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response fields: %v", err)
	}
	if _, ok := fields["works"]; !ok {
		t.Fatalf("response fields = %v, want snake_case works", fields)
	}
	if _, ok := fields["Works"]; ok {
		t.Fatalf("response leaked Go field name: %s", rec.Body.String())
	}
}

func TestWorkLedgerAPIListsWorksByIndexedSource(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	want, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: defaultWorkspaceID, Kind: "session", Source: "legacy-session", SourceID: "session-42",
		IdempotencyKey: "wanted", Title: "Wanted", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create wanted work: %v", err)
	}
	if _, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: defaultWorkspaceID, Kind: "session", Source: "legacy-session", SourceID: "session-other",
		IdempotencyKey: "other", Title: "Other", ActorID: "tester",
	}); err != nil {
		t.Fatalf("create other work: %v", err)
	}
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/work/works?source=legacy-session&source_id=session-42&limit=1", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("source-filtered list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Works []workstore.Work `json:"works"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode source-filtered list: %v", err)
	}
	if len(body.Works) != 1 || body.Works[0].ID != want.ID {
		t.Fatalf("source-filtered list = %#v", body.Works)
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/v1/work/works?source_id=session-42", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("source_id-only list status = %d body=%s, want 400", invalid.Code, invalid.Body.String())
	}
}

func TestWorkLedgerAPIReturnsReadOnlyTimelineProjection(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	work := createWorkLedgerHandlerTestWork(t, store, "workspace-a", "timeline", workstore.WorkStateReady)
	step, err := store.CreateStep(context.Background(), workstore.CreateStepInput{
		WorkspaceID:    "workspace-a",
		WorkID:         work.ID,
		IdempotencyKey: "step:one",
		Title:          "First step",
		State:          workstore.WorkStateReady,
		ActorID:        "tester",
	})
	if err != nil {
		t.Fatalf("create timeline step: %v", err)
	}
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/work/works/"+work.ID+"/timeline", nil)
	req = req.WithContext(serverauth.WithWorkspaceID(req.Context(), "workspace-a"))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("timeline status = %d body=%s", rec.Code, rec.Body.String())
	}
	var projection workstore.WorkProjection
	if err := json.Unmarshal(rec.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode timeline response: %v", err)
	}
	if projection.Work.ID != work.ID || len(projection.Steps) != 1 || projection.Steps[0].ID != step.ID {
		t.Fatalf("timeline projection = %#v", projection)
	}
	if len(projection.Events) != 2 || projection.Events[0].Sequence >= projection.Events[1].Sequence {
		t.Fatalf("timeline events are not ordered = %#v", projection.Events)
	}

	post := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/v1/work/works/"+work.ID+"/timeline", nil)
	postReq = postReq.WithContext(serverauth.WithWorkspaceID(postReq.Context(), "workspace-a"))
	handler.ServeHTTP(post, postReq)
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("timeline POST status = %d, want 405", post.Code)
	}

	otherWorkspace := httptest.NewRecorder()
	otherReq := httptest.NewRequest(http.MethodGet, "/v1/work/works/"+work.ID+"/timeline", nil)
	otherReq = otherReq.WithContext(serverauth.WithWorkspaceID(otherReq.Context(), "workspace-b"))
	handler.ServeHTTP(otherWorkspace, otherReq)
	if otherWorkspace.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace timeline status = %d, want 404", otherWorkspace.Code)
	}
}

func TestWorkLedgerAPIReturnsLegacyTasksProjection(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	want := []byte(`{"plan":{"goal":"Keep the API"},"tasks":[{"id":"task-1","title":"Projected","status":"completed"}]}`)
	if _, err := store.ImportLegacySession(context.Background(), workstore.LegacySessionImportInput{
		WorkspaceID: "workspace-a",
		SessionJSON: []byte(`{"id":"session-1","title":"Legacy"}`),
		TasksJSON:   want,
		SourcePath:  "/legacy/session-1.json",
		ActorID:     "migration",
	}); err != nil {
		t.Fatalf("import legacy tasks: %v", err)
	}
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/work/legacy/sessions/session-1/tasks", nil)
	req = req.WithContext(serverauth.WithWorkspaceID(req.Context(), "workspace-a"))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("legacy projection status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !handlerTestJSONEqual(rec.Body.Bytes(), want) {
		t.Fatalf("legacy projection = %s, want %s", rec.Body.Bytes(), want)
	}

	missing := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodGet, "/v1/work/legacy/sessions/missing/tasks", nil)
	missingReq = missingReq.WithContext(serverauth.WithWorkspaceID(missingReq.Context(), "workspace-a"))
	handler.ServeHTTP(missing, missingReq)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing legacy projection status = %d, want 404", missing.Code)
	}
}

func TestWorkLedgerAPIRejectsInvalidListQuery(t *testing.T) {
	t.Parallel()

	handler := newWorkLedgerAPIHandler(openWorkLedgerHandlerTestStore(t), zerolog.Nop())
	for _, target := range []string{
		"/v1/work/works?state=unknown",
		"/v1/work/works?limit=invalid",
		"/v1/work/works?offset=-1",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req = req.WithContext(serverauth.WithWorkspaceID(req.Context(), "workspace-a"))
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d body=%s, want 400", target, rec.Code, rec.Body.String())
		}
	}
}

func TestWorkLedgerAPIHandlesUnavailableUnknownAndMalformedRoutes(t *testing.T) {
	t.Parallel()

	unavailable := httptest.NewRecorder()
	newWorkLedgerAPIHandler(nil, zerolog.Nop()).ServeHTTP(
		unavailable,
		httptest.NewRequest(http.MethodGet, "/v1/work/works", nil),
	)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status = %d, want 503", unavailable.Code)
	}

	handler := newWorkLedgerAPIHandler(openWorkLedgerHandlerTestStore(t), zerolog.Nop())
	for _, target := range []string{
		"/v1/work/unknown",
		"/v1/work/works/work-only",
		"/v1/work/legacy/sessions/session-only",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", target, rec.Code)
		}
	}

	for _, target := range []string{
		"/v1/work/works/%zz/timeline",
		"/v1/work/legacy/sessions/%zz/tasks",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = target
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET malformed %s status = %d, want 400", target, rec.Code)
		}
	}
}

func TestWorkLedgerAPIReportsProjectionReadFailures(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("close work ledger before request: %v", err)
	}
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())
	for _, target := range []string{
		"/v1/work/works",
		"/v1/work/works/work-1/timeline",
		"/v1/work/legacy/sessions/session-1/tasks",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("GET %s status = %d body=%s, want 500", target, rec.Code, rec.Body.String())
		}
	}
}

func TestNormalizeWorkProjectionSlicesProducesStableEmptyArrays(t *testing.T) {
	t.Parallel()

	projection := workstore.WorkProjection{}
	normalizeWorkProjectionSlices(&projection)
	if projection.Steps == nil || projection.Dependencies == nil || projection.Attempts == nil ||
		projection.Events == nil || projection.Proofs == nil || projection.Artifacts == nil || projection.Approvals == nil {
		t.Fatalf("projection contains nil collection: %#v", projection)
	}
}

func openWorkLedgerHandlerTestStore(t *testing.T) *workstore.Store {
	t.Helper()
	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "work-ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close work ledger: %v", err)
		}
	})
	return store
}

func createWorkLedgerHandlerTestWork(t *testing.T, store *workstore.Store, workspaceID, key string, state workstore.WorkState) workstore.Work {
	t.Helper()
	work, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID:    workspaceID,
		Kind:           "task",
		IdempotencyKey: key,
		Title:          key,
		InitialState:   state,
		ActorID:        "tester",
	})
	if err != nil {
		t.Fatalf("create test work: %v", err)
	}
	return work
}

func handlerTestJSONEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil &&
		jsonValuesEqual(leftValue, rightValue)
}

func jsonValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
