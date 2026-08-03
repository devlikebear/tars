package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestSessionTasksGETUsesWorkLedgerProjectionWithLegacyContract(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	legacyTasks := session.SessionTasks{
		Plan:  &session.Plan{Goal: "Legacy file", Status: session.PlanStatusExecuting},
		Tasks: []session.Task{{ID: "1", Title: "Legacy task", Status: "pending"}},
	}
	if err := sessionStore.SaveTasks(sess.ID, legacyTasks); err != nil {
		t.Fatalf("save legacy tasks: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	projectedJSON := []byte(`{"plan":{"goal":"Ledger projection","created_at":"","status":"executing"},"tasks":[{"id":"2","title":"Projected task","status":"in_progress"}]}`)
	sessionJSON, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if _, err := ledger.ImportLegacySession(context.Background(), workstore.LegacySessionImportInput{
		WorkspaceID: defaultWorkspaceID,
		SessionJSON: sessionJSON,
		TasksJSON:   projectedJSON,
		ActorID:     "test-import",
	}); err != nil {
		t.Fatalf("import projected tasks: %v", err)
	}
	handler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())

	rec := sessionTasksRequest(t, handler, http.MethodGet, sess.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET projected tasks status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got session.SessionTasks
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode projected tasks: %v", err)
	}
	if got.Plan == nil || got.Plan.Goal != "Ledger projection" || len(got.Tasks) != 1 || got.Tasks[0].ID != "2" {
		t.Fatalf("projected tasks response = %#v", got)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response contract: %v", err)
	}
	if _, ok := raw["tasks"]; !ok {
		t.Fatalf("legacy response contract lost tasks array: %s", rec.Body.String())
	}
	if _, err := ledger.ImportLegacySession(context.Background(), workstore.LegacySessionImportInput{
		WorkspaceID: defaultWorkspaceID,
		SessionJSON: sessionJSON,
		TasksJSON:   []byte(`{"plan":{"goal":"Normalized empty"},"tasks":null}`),
		ActorID:     "test-import",
	}); err != nil {
		t.Fatalf("import empty projected tasks: %v", err)
	}
	empty := sessionTasksRequest(t, handler, http.MethodGet, sess.ID, "")
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"tasks":[]`) {
		t.Fatalf("normalized empty projection status=%d body=%s", empty.Code, empty.Body.String())
	}
}

func TestSessionTasksGETFallsBackWhenLedgerProjectionUnavailable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	want := session.SessionTasks{
		Plan:  &session.Plan{Goal: "Legacy fallback", Status: session.PlanStatusExecuting},
		Tasks: []session.Task{{ID: "1", Title: "Fallback task", Status: "pending"}},
	}
	if err := sessionStore.SaveTasks(sess.ID, want); err != nil {
		t.Fatalf("save fallback tasks: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	missingProjectionHandler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())
	missingProjection := sessionTasksRequest(t, missingProjectionHandler, http.MethodGet, sess.ID, "")
	if missingProjection.Code != http.StatusOK || !strings.Contains(missingProjection.Body.String(), "Legacy fallback") {
		t.Fatalf("GET missing projection fallback status=%d body=%s", missingProjection.Code, missingProjection.Body.String())
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	handler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())

	rec := sessionTasksRequest(t, handler, http.MethodGet, sess.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET fallback tasks status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got session.SessionTasks
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode fallback tasks: %v", err)
	}
	if got.Plan == nil || got.Plan.Goal != want.Plan.Goal || len(got.Tasks) != 1 || got.Tasks[0].ID != "1" {
		t.Fatalf("fallback tasks response = %#v", got)
	}

	post := sessionTasksRequest(t, handler, http.MethodPost, sess.ID, `{"action":"plan_set","goal":"Legacy still succeeds"}`)
	if post.Code != http.StatusOK {
		t.Fatalf("POST with unavailable ledger status = %d body=%s", post.Code, post.Body.String())
	}
	persisted, err := sessionStore.GetTasks(sess.ID)
	if err != nil || persisted.Plan == nil || persisted.Plan.Goal != "Legacy still succeeds" {
		t.Fatalf("legacy mutation after ledger failure tasks=%#v err=%v", persisted, err)
	}
}

func TestSessionTaskVerificationSynchronizesEvidenceToWorkLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	if err := sessionStore.SaveTasks(sess.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Verify", Status: session.PlanStatusExecuting},
		Contract: &session.TaskContract{
			Goal:                 "Verify",
			Status:               session.ContractStatusApproved,
			VerificationCommands: []string{"printf ledger-verification"},
		},
		Tasks: []session.Task{{ID: "1", Title: "Verify work", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save verification tasks: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	handler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify tasks status = %d body=%s", rec.Code, rec.Body.String())
	}

	projection, found, err := ledger.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, sess.ID)
	if err != nil || !found {
		t.Fatalf("get verified projection found=%t err=%v", found, err)
	}
	var projected session.SessionTasks
	if err := json.Unmarshal(projection, &projected); err != nil {
		t.Fatalf("decode verified projection: %v", err)
	}
	if len(projected.Tasks) != 1 || len(projected.Tasks[0].Evidence) != 1 || projected.Tasks[0].Evidence[0].Status != "passed" {
		t.Fatalf("verified projection = %#v", projected)
	}
	works, err := ledger.ListWorks(context.Background(), workstore.ListWorksFilter{
		WorkspaceID: defaultWorkspaceID, Source: string(workstore.ImportSourceLegacySession), SourceID: sess.ID, Limit: 10,
	})
	if err != nil || len(works) == 0 {
		t.Fatalf("list verified work revisions = %d, %v", len(works), err)
	}
	verifiedWork, err := ledger.GetWorkProjection(context.Background(), defaultWorkspaceID, works[0].ID)
	if err != nil || len(verifiedWork.Proofs) != 1 {
		t.Fatalf("get independently verified ledger proof = %+v, %v", verifiedWork.Proofs, err)
	}
	proof := verifiedWork.Proofs[0]
	if proof.Status != workstore.ProofStatusPassed || proof.Origin != workstore.ProofOriginIndependentVerifier || proof.ReporterID == proof.VerifierID || proof.SubjectDigest == "" {
		t.Fatalf("verified ledger proof = %+v", proof)
	}
}

func TestSyncSessionTasksToWorkLedgerHandlesDisabledAndMissingStores(t *testing.T) {
	t.Parallel()

	if err := syncSessionTasksToWorkLedger(context.Background(), nil, nil, defaultWorkspaceID, "missing"); err != nil {
		t.Fatalf("disabled sync: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	if err := syncSessionTasksToWorkLedger(context.Background(), ledger, session.NewStore(t.TempDir()), defaultWorkspaceID, "missing"); err == nil {
		t.Fatal("expected missing session sync to fail")
	}
}

func TestSessionTasksPOSTSynchronizesSuccessfulMutationToWorkLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	handler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())

	rec := sessionTasksRequest(t, handler, http.MethodPost, sess.ID, `{"action":"plan_set","goal":"Synchronized plan"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST tasks status = %d body=%s", rec.Code, rec.Body.String())
	}
	projection, found, err := ledger.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, sess.ID)
	if err != nil || !found {
		t.Fatalf("get synchronized projection found=%t err=%v", found, err)
	}
	var projected session.SessionTasks
	if err := json.Unmarshal(projection, &projected); err != nil {
		t.Fatalf("decode synchronized projection: %v", err)
	}
	if projected.Plan == nil || projected.Plan.Goal != "Synchronized plan" || projected.Plan.Status != session.PlanStatusDrafting {
		t.Fatalf("synchronized projection = %#v", projected)
	}

	get := sessionTasksRequest(t, handler, http.MethodGet, sess.ID, "")
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "Synchronized plan") {
		t.Fatalf("GET synchronized tasks status=%d body=%s", get.Code, get.Body.String())
	}
}

func TestTasksToolDirectSaveSynchronizesToWorkLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	_ = newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())
	tasksTool := tool.NewTasksTool(sessionStore, root, func() string { return sess.ID })

	for _, payload := range []string{
		`{"action":"plan_set","goal":"Tool-synchronized plan"}`,
		`{"action":"add","title":"Persist tool mutation"}`,
	} {
		result, executeErr := tasksTool.Execute(context.Background(), json.RawMessage(payload))
		if executeErr != nil || result.IsError {
			t.Fatalf("execute tasks tool payload=%s result=%s err=%v", payload, result.Text(), executeErr)
		}
	}

	projection, found, err := ledger.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, sess.ID)
	if err != nil || !found {
		t.Fatalf("get tool-synchronized projection found=%t err=%v", found, err)
	}
	var projected session.SessionTasks
	if err := json.Unmarshal(projection, &projected); err != nil {
		t.Fatalf("decode tool-synchronized projection: %v", err)
	}
	if projected.Plan == nil || projected.Plan.Goal != "Tool-synchronized plan" || len(projected.Tasks) != 1 || projected.Tasks[0].Title != "Persist tool mutation" {
		t.Fatalf("tool-synchronized projection = %#v", projected)
	}
}

func sessionTasksRequest(t *testing.T, handler http.Handler, method, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/v1/admin/sessions/"+sessionID+"/tasks", nil)
	} else {
		req = httptest.NewRequest(method, "/v1/admin/sessions/"+sessionID+"/tasks", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
