package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestSyncAgentRuntimeRunsToWorkLedgerTracksLatestRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ledger := openWorkLedgerHandlerTestStore(t)
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	run := agentruntime.Run{
		ID: "run-1", SessionID: "session-1", Prompt: "Build durable runtime",
		Status: agentruntime.RunStatusRunning, Accepted: true,
		CreatedAt: "2026-08-02T01:00:00Z", UpdatedAt: "2026-08-02T01:01:00Z",
	}
	if err := syncAgentRuntimeRunsToWorkLedger(ctx, ledger, defaultWorkspaceID, runsPath, []agentruntime.Run{run}, "runtime-sync"); err != nil {
		t.Fatalf("sync running snapshot: %v", err)
	}
	run.Status = agentruntime.RunStatusCompleted
	run.Response = "done"
	run.CompletedAt = "2026-08-02T01:02:00Z"
	run.UpdatedAt = run.CompletedAt
	if err := syncAgentRuntimeRunsToWorkLedger(ctx, ledger, defaultWorkspaceID, runsPath, []agentruntime.Run{run}, "runtime-sync"); err != nil {
		t.Fatalf("sync completed snapshot: %v", err)
	}
	if err := syncAgentRuntimeRunsToWorkLedger(ctx, ledger, defaultWorkspaceID, runsPath, []agentruntime.Run{run}, "runtime-sync"); err != nil {
		t.Fatalf("replay completed snapshot: %v", err)
	}

	raw, found, err := ledger.GetLegacyAgentRuntimeRunProjection(ctx, defaultWorkspaceID, run.ID)
	if err != nil || !found {
		t.Fatalf("get synchronized run projection found=%t err=%v", found, err)
	}
	var projected agentruntime.Run
	if err := json.Unmarshal(raw, &projected); err != nil {
		t.Fatalf("decode synchronized run: %v", err)
	}
	if projected.Status != agentruntime.RunStatusCompleted || projected.Response != "done" {
		t.Fatalf("synchronized run = %#v", projected)
	}
	works, err := ledger.ListWorks(ctx, workstore.ListWorksFilter{WorkspaceID: defaultWorkspaceID, Source: string(workstore.ImportSourceAgentRuntime)})
	if err != nil || len(works) != 2 {
		t.Fatalf("runtime revision works = %d err=%v, want 2", len(works), err)
	}
	if err := syncAgentRuntimeRunsToWorkLedger(ctx, nil, defaultWorkspaceID, runsPath, nil, "runtime-sync"); err != nil {
		t.Fatalf("disabled runtime sync: %v", err)
	}
}

func TestAgentRunsAPIReadsLedgerProjectionWithoutRuntime(t *testing.T) {
	t.Parallel()

	ledger := openWorkLedgerHandlerTestStore(t)
	if _, err := ledger.ImportAgentRuntimeSnapshot(context.Background(), workstore.AgentRuntimeImportInput{
		WorkspaceID: defaultWorkspaceID,
		SourceID:    "runs",
		SnapshotJSON: []byte(`{"runs":[
			{"run_id":"run-old","prompt":"Older","status":"failed","updated_at":"2026-08-02T01:00:00Z"},
			{"run_id":"run-done","prompt":"Verify ledger API","status":"completed","response":"verified","updated_at":"2026-08-02T01:01:00Z"}
		]}`),
		ActorID: "bootstrap",
	}); err != nil {
		t.Fatalf("import runtime projection: %v", err)
	}
	handler := newAgentRunsAPIHandlerWithWorkLedger(nil, ledger, zerolog.New(io.Discard))

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs?status=completed", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("ledger run list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Count int                `json:"count"`
		Runs  []agentruntime.Run `json:"runs"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode ledger run list: %v", err)
	}
	if listed.Count != 1 || len(listed.Runs) != 1 || listed.Runs[0].ID != "run-done" {
		t.Fatalf("ledger run list = %#v", listed)
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs/run-done", nil))
	if detail.Code != http.StatusOK || !json.Valid(detail.Body.Bytes()) {
		t.Fatalf("ledger run detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var projected agentruntime.Run
	if err := json.Unmarshal(detail.Body.Bytes(), &projected); err != nil || projected.Response != "verified" {
		t.Fatalf("ledger run detail = %#v err=%v", projected, err)
	}

	cancel := httptest.NewRecorder()
	handler.ServeHTTP(cancel, httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs/run-done/cancel", nil))
	if cancel.Code != http.StatusServiceUnavailable {
		t.Fatalf("ledger-only cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}

	restart := httptest.NewRecorder()
	handler.ServeHTTP(restart, httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs/run-done/restart", nil))
	if restart.Code != http.StatusServiceUnavailable {
		t.Fatalf("ledger-only restart status=%d body=%s", restart.Code, restart.Body.String())
	}
}

func TestAgentRunsAPIReturnsLedgerReadErrorsWithoutRuntime(t *testing.T) {
	t.Parallel()

	ledger := openWorkLedgerHandlerTestStore(t)
	handler := newAgentRunsAPIHandlerWithWorkLedger(nil, ledger, zerolog.New(io.Discard))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs", nil).WithContext(canceled)
	handler.ServeHTTP(list, listRequest)
	if list.Code != http.StatusInternalServerError {
		t.Fatalf("canceled ledger list status=%d body=%s", list.Code, list.Body.String())
	}

	detail := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs/run-missing", nil).WithContext(canceled)
	handler.ServeHTTP(detail, detailRequest)
	if detail.Code != http.StatusInternalServerError {
		t.Fatalf("canceled ledger detail status=%d body=%s", detail.Code, detail.Body.String())
	}
}

func TestAgentRunsAPIPrefersLiveRuntimeOverLedgerRevision(t *testing.T) {
	t.Parallel()

	runtime := newTestAgentRuntime(t)
	ledger := openWorkLedgerHandlerTestStore(t)
	if _, err := ledger.ImportAgentRuntimeSnapshot(context.Background(), workstore.AgentRuntimeImportInput{
		WorkspaceID: defaultWorkspaceID,
		SourceID:    "runs",
		SnapshotJSON: []byte(`{"runs":[
			{"run_id":"run_1","prompt":"Stale","status":"failed","error":"stale ledger revision","updated_at":"2026-08-02T01:00:00Z"}
		]}`),
		ActorID: "bootstrap",
	}); err != nil {
		t.Fatalf("import stale runtime projection: %v", err)
	}
	live, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "Live runtime wins"})
	if err != nil {
		t.Fatalf("spawn live runtime: %v", err)
	}
	waitForAgentRuntimeRun(t, runtime, live.ID)
	handler := newAgentRunsAPIHandlerWithWorkLedger(runtime, ledger, zerolog.New(io.Discard))

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs/"+live.ID, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("live run detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var projected agentruntime.Run
	if err := json.Unmarshal(detail.Body.Bytes(), &projected); err != nil {
		t.Fatalf("decode live run detail: %v", err)
	}
	if projected.Status != agentruntime.RunStatusCompleted || projected.Error != "" || projected.Prompt != "Live runtime wins" {
		t.Fatalf("live runtime was not preferred: %#v", projected)
	}
}
