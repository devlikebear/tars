package workstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestImportLegacySessionPreservesRecordsAndIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	sessionJSON := []byte(`{
		"id":"session-42",
		"title":"Durable migration",
		"kind":"main",
		"current_dir":"/workspace/tars",
		"goal":{
			"description":"Migrate without loss",
			"created_at":"2026-08-01T01:00:00Z",
			"max_auto_continues":3,
			"auto_continue_count":1,
			"status":"satisfied"
		},
		"created_at":"2026-08-01T00:00:00Z",
		"updated_at":"2026-08-01T02:00:00Z"
	}`)
	tasksJSON := []byte(`{
		"plan":{
			"goal":"Ship the ledger",
			"constraints":"local-first",
			"created_at":"2026-08-01T01:05:00Z",
			"status":"completed",
			"updated_at":"2026-08-01T02:00:00Z"
		},
		"contract":{
			"goal":"Ship the ledger",
			"scope":"storage only",
			"done_criteria":["tests pass"],
			"verification_commands":["go test ./..."],
			"artifacts":["coverage.out"],
			"status":"approved"
		},
		"tasks":[
			{
				"id":"task-1",
				"title":"Create schema",
				"status":"completed",
				"description":"Persist all records",
				"run_id":"run_1",
				"evidence":[{
					"id":"evidence-1",
					"type":"test_result",
					"title":"Unit tests",
					"summary":"all tests passed",
					"command":"go test ./internal/workstore",
					"path":"coverage.out",
					"status":"passed",
					"created_at":"2026-08-01T01:30:00Z"
				}]
			},
			{
				"id":"task-2",
				"title":"Wire API",
				"status":"in_progress",
				"description":"Expose projections"
			}
		]
	}`)
	originalSession := append([]byte(nil), sessionJSON...)
	originalTasks := append([]byte(nil), tasksJSON...)

	result, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a",
		SessionJSON: sessionJSON,
		TasksJSON:   tasksJSON,
		SourcePath:  "/legacy/sessions/session-42.json",
		ActorID:     "migration",
	})
	if err != nil {
		t.Fatalf("import legacy session: %v", err)
	}
	if result.AlreadyImported || result.Marker.Status != ImportStatusCompleted || len(result.WorkIDs) != 1 {
		t.Fatalf("first import result = %#v", result)
	}
	if result.Marker.SourceKind != ImportSourceLegacySession || result.Marker.SourceID != "session-42" || result.Marker.Checksum == "" {
		t.Fatalf("import marker = %#v", result.Marker)
	}
	if !bytes.Equal(sessionJSON, originalSession) || !bytes.Equal(tasksJSON, originalTasks) {
		t.Fatal("import mutated its source buffers")
	}

	projection, err := store.GetWorkProjection(ctx, "workspace-a", result.WorkIDs[0])
	if err != nil {
		t.Fatalf("get imported projection: %v", err)
	}
	if projection.Work.Source != string(ImportSourceLegacySession) || projection.Work.SourceID != "session-42" || projection.Work.State != WorkStateDone {
		t.Fatalf("imported work provenance/state = %s/%s/%s", projection.Work.Source, projection.Work.SourceID, projection.Work.State)
	}
	if projection.Work.Objective != "Migrate without loss" || len(projection.Steps) != 2 {
		t.Fatalf("imported objective/steps = %q/%d", projection.Work.Objective, len(projection.Steps))
	}
	if projection.Steps[0].State != WorkStateDone || projection.Steps[1].State != WorkStateRunning {
		t.Fatalf("imported step states = %s, %s", projection.Steps[0].State, projection.Steps[1].State)
	}
	if len(projection.Artifacts) != 1 || len(projection.Proofs) != 1 {
		t.Fatalf("imported evidence counts = artifacts:%d proofs:%d", len(projection.Artifacts), len(projection.Proofs))
	}
	if projection.Proofs[0].Status != ProofStatusPassed || projection.Proofs[0].Command != "go test ./internal/workstore" {
		t.Fatalf("imported proof = %#v", projection.Proofs[0])
	}
	if !jsonEqual(projection.Work.ContractJSON, []byte(`{
		"goal":"Ship the ledger",
		"scope":"storage only",
		"done_criteria":["tests pass"],
		"verification_commands":["go test ./..."],
		"artifacts":["coverage.out"],
		"status":"approved"
	}`)) {
		t.Fatalf("imported contract = %s", projection.Work.ContractJSON)
	}
	var metadata struct {
		Session json.RawMessage `json:"legacy_session"`
		Tasks   json.RawMessage `json:"legacy_tasks"`
	}
	if err := json.Unmarshal(projection.Work.MetadataJSON, &metadata); err != nil {
		t.Fatalf("decode import metadata: %v", err)
	}
	if !jsonEqual(metadata.Session, sessionJSON) || !jsonEqual(metadata.Tasks, tasksJSON) {
		t.Fatal("root metadata did not preserve the complete source documents")
	}

	replayed, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a",
		SessionJSON: sessionJSON,
		TasksJSON:   tasksJSON,
		SourcePath:  "/legacy/sessions/session-42.json",
		ActorID:     "migration",
	})
	if err != nil {
		t.Fatalf("replay legacy import: %v", err)
	}
	if !replayed.AlreadyImported || replayed.Marker.ID != result.Marker.ID || replayed.WorkIDs[0] != result.WorkIDs[0] {
		t.Fatalf("replayed import result = %#v, first = %#v", replayed, result)
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list imported works: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("work count after replay = %d, want 1", len(works))
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("list import markers: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("marker count after replay = %d, want 1", len(markers))
	}
}

func TestImportLegacySessionRecoversAfterInterruptedCommitSequence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	idCalls := 0
	interrupted, err := Open(ctx, path, Options{
		NewID: func(prefix string) (string, error) {
			idCalls++
			if idCalls == 5 {
				return "", fmt.Errorf("simulated process interruption")
			}
			return fmt.Sprintf("%s_interrupted_%d", prefix, idCalls), nil
		},
	})
	if err != nil {
		t.Fatalf("open interrupted store: %v", err)
	}
	input := LegacySessionImportInput{
		WorkspaceID: "workspace-crash",
		SessionJSON: []byte(`{"id":"session-crash","title":"Recover import"}`),
		TasksJSON: []byte(`{"plan":{"goal":"Converge after restart"},"tasks":[
			{"id":"task-1","title":"First committed step","status":"completed"},
			{"id":"task-2","title":"Resume remaining step","status":"pending"}
		]}`),
		SourcePath: "/legacy/session-crash.tasks.json",
		ActorID:    "migration",
	}
	if _, err := interrupted.ImportLegacySession(ctx, input); err == nil {
		t.Fatal("expected injected interruption to fail import")
	}
	partialWorks, err := interrupted.ListWorks(ctx, ListWorksFilter{WorkspaceID: input.WorkspaceID})
	if err != nil || len(partialWorks) != 1 {
		t.Fatalf("partial works=%d err=%v, want one committed work", len(partialWorks), err)
	}
	partialProjection, err := interrupted.GetWorkProjection(ctx, input.WorkspaceID, partialWorks[0].ID)
	if err != nil || len(partialProjection.Steps) != 1 {
		t.Fatalf("partial projection steps=%d err=%v, want one committed step", len(partialProjection.Steps), err)
	}
	partialMarkers, err := interrupted.ListImportMarkers(ctx, input.WorkspaceID)
	if err != nil || len(partialMarkers) != 0 {
		t.Fatalf("partial markers=%d err=%v, want no completed marker", len(partialMarkers), err)
	}
	if err := interrupted.Close(); err != nil {
		t.Fatalf("close interrupted store: %v", err)
	}

	reopened, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("reopen interrupted store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	result, err := reopened.ImportLegacySession(ctx, input)
	if err != nil {
		t.Fatalf("resume interrupted import: %v", err)
	}
	if result.AlreadyImported || len(result.WorkIDs) != 1 || result.WorkIDs[0] != partialWorks[0].ID {
		t.Fatalf("resumed import result = %+v", result)
	}
	projection, err := reopened.GetWorkProjection(ctx, input.WorkspaceID, result.WorkIDs[0])
	if err != nil {
		t.Fatalf("get resumed projection: %v", err)
	}
	if len(projection.Steps) != 2 || projection.Steps[0].Title != "First committed step" || projection.Steps[1].Title != "Resume remaining step" {
		t.Fatalf("resumed projection did not converge: %+v", projection.Steps)
	}
	works, err := reopened.ListWorks(ctx, ListWorksFilter{WorkspaceID: input.WorkspaceID})
	if err != nil || len(works) != 1 {
		t.Fatalf("resumed works=%d err=%v, want no duplicate", len(works), err)
	}
	markers, err := reopened.ListImportMarkers(ctx, input.WorkspaceID)
	if err != nil || len(markers) != 1 || markers[0].Status != ImportStatusCompleted {
		t.Fatalf("resumed markers=%+v err=%v", markers, err)
	}
	report, err := reopened.Doctor(ctx, input.WorkspaceID)
	if err != nil || !report.Healthy {
		t.Fatalf("doctor after resumed import healthy=%t issues=%+v err=%v", report.Healthy, report.Issues, err)
	}
}

func FuzzImportLegacySessionCommitSafety(f *testing.F) {
	f.Add(
		[]byte(`{"id":"session-fuzz","title":"Valid seed"}`),
		[]byte(`{"tasks":[{"id":"task-1","title":"Seed task","status":"pending"}]}`),
	)
	f.Add([]byte(`{`), []byte(`{"tasks":[]}`))
	f.Add([]byte(`{"id":"session-fuzz"}`), []byte(`{"tasks":[`))

	f.Fuzz(func(t *testing.T, sessionJSON, tasksJSON []byte) {
		if len(sessionJSON) > 4096 || len(tasksJSON) > 8192 {
			t.Skip()
		}
		ctx := context.Background()
		store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
		input := LegacySessionImportInput{
			WorkspaceID: "workspace-fuzz",
			SessionJSON: sessionJSON,
			TasksJSON:   tasksJSON,
			ActorID:     "fuzzer",
		}
		result, importErr := store.ImportLegacySession(ctx, input)
		works, listErr := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: input.WorkspaceID})
		if listErr != nil {
			t.Fatalf("list works after fuzz import: %v", listErr)
		}
		markers, markerErr := store.ListImportMarkers(ctx, input.WorkspaceID)
		if markerErr != nil {
			t.Fatalf("list markers after fuzz import: %v", markerErr)
		}
		if importErr != nil {
			if len(works) != 0 || len(markers) != 0 {
				t.Fatalf("rejected fuzz input left partial state: works=%d markers=%d err=%v", len(works), len(markers), importErr)
			}
			return
		}
		if len(result.WorkIDs) != 1 || len(works) != 1 || len(markers) != 1 {
			t.Fatalf("accepted fuzz input produced incoherent state: result=%+v works=%d markers=%d", result, len(works), len(markers))
		}
		replayed, replayErr := store.ImportLegacySession(ctx, input)
		if replayErr != nil || !replayed.AlreadyImported || replayed.Marker.ID != result.Marker.ID {
			t.Fatalf("accepted fuzz input did not replay idempotently: replay=%+v err=%v", replayed, replayErr)
		}
		report, doctorErr := store.Doctor(ctx, input.WorkspaceID)
		if doctorErr != nil || !report.Healthy {
			t.Fatalf("accepted fuzz input failed doctor: healthy=%t issues=%+v err=%v", report.Healthy, report.Issues, doctorErr)
		}
	})
}

func TestGetLegacySessionTasksProjectionReturnsLatestWorkspaceScopedRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	sessionJSON := []byte(`{"id":"session-42","title":"Projection source"}`)
	firstTasks := []byte(`{"plan":{"goal":"First"},"tasks":[{"id":"task-1","title":"First task","status":"pending"}]}`)
	latestTasks := []byte(`{"plan":{"goal":"Latest"},"contract":{"status":"approved"},"tasks":[{"id":"task-2","title":"Latest task","status":"in_progress"}]}`)

	for _, tasksJSON := range [][]byte{firstTasks, latestTasks} {
		if _, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
			WorkspaceID: "workspace-a",
			SessionJSON: sessionJSON,
			TasksJSON:   tasksJSON,
			SourcePath:  "/legacy/session-42.json",
			ActorID:     "migration",
		}); err != nil {
			t.Fatalf("import legacy session revision: %v", err)
		}
	}

	projection, found, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "session-42")
	if err != nil {
		t.Fatalf("get latest legacy tasks projection: %v", err)
	}
	if !found {
		t.Fatal("expected latest legacy tasks projection")
	}
	if !jsonEqual(projection, latestTasks) {
		t.Fatalf("latest projection = %s, want %s", projection, latestTasks)
	}

	if _, found, err := store.GetLegacySessionTasksProjection(ctx, "workspace-b", "session-42"); err != nil || found {
		t.Fatalf("cross-workspace projection found=%t err=%v, want false/nil", found, err)
	}
	if _, found, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "missing"); err != nil || found {
		t.Fatalf("missing projection found=%t err=%v, want false/nil", found, err)
	}
}

func TestGetLegacySessionTasksProjectionRejectsInvalidInputsAndMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	if _, _, err := store.GetLegacySessionTasksProjection(ctx, "", "session-1"); err == nil {
		t.Fatal("expected empty workspace id to fail")
	}
	result, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a",
		SessionJSON: []byte(`{"id":"session-1","title":"Invalid metadata"}`),
		TasksJSON:   []byte(`{"tasks":[]}`),
		ActorID:     "migration",
	})
	if err != nil {
		t.Fatalf("import legacy session: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE works SET metadata_json = ? WHERE id = ?", []byte(`not-json`), result.WorkIDs[0]); err != nil {
		t.Fatalf("corrupt projection metadata: %v", err)
	}
	if _, _, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "session-1"); err == nil {
		t.Fatal("expected malformed projection metadata to fail")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE works SET metadata_json = ? WHERE id = ?", []byte(`{"legacy_tasks":null}`), result.WorkIDs[0]); err != nil {
		t.Fatalf("remove legacy tasks projection: %v", err)
	}
	if _, _, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "session-1"); err == nil {
		t.Fatal("expected missing legacy tasks projection to fail")
	}
}

func TestGetLegacySessionTasksProjectionUsesInsertionOrderWhenTimestampsTie(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	counts := map[string]int{}
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), Options{
		Now: func() time.Time { return time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			counts[prefix]++
			if prefix == "wrk" {
				if counts[prefix] == 1 {
					return "wrk_z_older", nil
				}
				return "wrk_a_latest", nil
			}
			return fmt.Sprintf("%s_%d", prefix, counts[prefix]), nil
		},
	})
	if err != nil {
		t.Fatalf("open work store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sessionJSON := []byte(`{"id":"session-tie","title":"Tied timestamp"}`)
	older := []byte(`{"plan":{"goal":"Older"},"tasks":[]}`)
	latest := []byte(`{"plan":{"goal":"Latest"},"tasks":[]}`)
	for _, tasksJSON := range [][]byte{older, latest} {
		if _, err := store.ImportLegacySession(context.Background(), LegacySessionImportInput{
			WorkspaceID: "workspace-a", SessionJSON: sessionJSON, TasksJSON: tasksJSON, ActorID: "migration",
		}); err != nil {
			t.Fatalf("import tied revision: %v", err)
		}
	}

	projection, found, err := store.GetLegacySessionTasksProjection(context.Background(), "workspace-a", "session-tie")
	if err != nil || !found {
		t.Fatalf("get tied latest projection found=%t err=%v", found, err)
	}
	if !jsonEqual(projection, latest) {
		t.Fatalf("tied latest projection = %s, want %s", projection, latest)
	}
}

func TestImportLegacySessionCreatesNewAppendOnlySnapshotWhenSourceChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	sessionJSON := []byte(`{"id":"session-1","title":"Session","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}`)
	firstTasks := []byte(`{"tasks":[{"id":"task-1","title":"First","status":"pending"}]}`)
	secondTasks := []byte(`{"tasks":[{"id":"task-1","title":"First","status":"completed"},{"id":"task-2","title":"Second","status":"pending"}]}`)

	first, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: sessionJSON, TasksJSON: firstTasks,
		SourcePath: "/legacy/session-1.json", ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("import first snapshot: %v", err)
	}
	second, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: sessionJSON, TasksJSON: secondTasks,
		SourcePath: "/legacy/session-1.json", ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("import changed snapshot: %v", err)
	}
	if first.Marker.Checksum == second.Marker.Checksum || first.WorkIDs[0] == second.WorkIDs[0] {
		t.Fatalf("changed source did not create an append-only revision: first=%#v second=%#v", first, second)
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list works: %v", err)
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("list markers: %v", err)
	}
	if len(works) != 2 || len(markers) != 2 {
		t.Fatalf("append-only counts = works:%d markers:%d, want 2/2", len(works), len(markers))
	}
}

func TestImportAgentRuntimeSnapshotPreservesRunAttempts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	snapshotJSON := []byte(`{
		"runs":[
			{
				"run_id":"run_1",
				"session_id":"session-42",
				"task_id":"task-1",
				"agent":"builder",
				"prompt":"Implement schema",
				"status":"completed",
				"accepted":true,
				"response":"schema ready",
				"created_at":"2026-08-01T01:10:00Z",
				"started_at":"2026-08-01T01:11:00Z",
				"completed_at":"2026-08-01T01:12:00Z",
				"updated_at":"2026-08-01T01:12:00Z"
			},
			{
				"run_id":"run_2",
				"session_id":"session-42",
				"parent_run_id":"run_1",
				"agent":"reviewer",
				"resolved_kind":"openai",
				"prompt":"Review schema",
				"status":"running",
				"accepted":true,
				"created_at":"2026-08-01T01:13:00Z",
				"started_at":"2026-08-01T01:14:00Z",
				"updated_at":"2026-08-01T01:14:00Z"
			}
		]
	}`)

	result, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID:  "workspace-a",
		SourceID:     "runtime-primary",
		SourcePath:   "/legacy/agentruntime/runs.json",
		SnapshotJSON: snapshotJSON,
		ActorID:      "migration",
	})
	if err != nil {
		t.Fatalf("import agent runtime snapshot: %v", err)
	}
	if result.AlreadyImported || result.Marker.SourceKind != ImportSourceAgentRuntime || len(result.WorkIDs) != 2 {
		t.Fatalf("runtime import result = %#v", result)
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list runtime works: %v", err)
	}
	if len(works) != 2 {
		t.Fatalf("runtime work count = %d, want 2", len(works))
	}
	bySource := make(map[string]Work, len(works))
	for _, work := range works {
		bySource[work.SourceID] = work
	}
	completed := bySource["run_1"]
	running := bySource["run_2"]
	if completed.State != WorkStateDone || running.State != WorkStateRunning {
		t.Fatalf("runtime work states = completed:%s running:%s", completed.State, running.State)
	}
	if running.ParentWorkID != completed.ID {
		t.Fatalf("runtime parent work = %q, want %q", running.ParentWorkID, completed.ID)
	}
	completedProjection, err := store.GetWorkProjection(ctx, completed.WorkspaceID, completed.ID)
	if err != nil {
		t.Fatalf("get completed run projection: %v", err)
	}
	if len(completedProjection.Steps) != 1 || len(completedProjection.Attempts) != 1 {
		t.Fatalf("completed run projection counts = steps:%d attempts:%d", len(completedProjection.Steps), len(completedProjection.Attempts))
	}
	completedAttempt := completedProjection.Attempts[0]
	if completedAttempt.Status != AttemptStatusSucceeded || completedAttempt.ErrorText != "" || completedAttempt.FinishedAt == nil {
		t.Fatalf("completed attempt = %#v", completedAttempt)
	}
	if completedAttempt.StartedAt != time.Date(2026, time.August, 1, 1, 11, 0, 0, time.UTC) ||
		*completedAttempt.FinishedAt != time.Date(2026, time.August, 1, 1, 12, 0, 0, time.UTC) {
		t.Fatalf("completed attempt timestamps = %s/%v", completedAttempt.StartedAt, completedAttempt.FinishedAt)
	}
	var output struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(completedAttempt.OutputJSON, &output); err != nil || output.Response != "schema ready" {
		t.Fatalf("completed attempt output = %s, %v", completedAttempt.OutputJSON, err)
	}
	runningProjection, err := store.GetWorkProjection(ctx, running.WorkspaceID, running.ID)
	if err != nil {
		t.Fatalf("get running run projection: %v", err)
	}
	if runningProjection.Attempts[0].Status != AttemptStatusRunning || runningProjection.Attempts[0].FinishedAt != nil || runningProjection.Attempts[0].Adapter != "openai" {
		t.Fatalf("running attempt = %#v", runningProjection.Attempts[0])
	}

	replayed, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runtime-primary",
		SourcePath: "/legacy/agentruntime/runs.json", SnapshotJSON: snapshotJSON,
		ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("replay runtime snapshot: %v", err)
	}
	if !replayed.AlreadyImported || replayed.Marker.ID != result.Marker.ID {
		t.Fatalf("replayed runtime result = %#v", replayed)
	}
	works, err = store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil || len(works) != 2 {
		t.Fatalf("runtime works after replay = %d, %v", len(works), err)
	}
}

func TestLegacyAgentRuntimeRunProjectionsReturnLatestRunRevisions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	first, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a",
		SourceID:    "runs",
		SnapshotJSON: []byte(`{"runs":[
			{"run_id":"run-1","prompt":"Build","status":"running","updated_at":"2026-08-02T01:00:00Z"}
		]}`),
		ActorID: "runtime-sync",
	})
	if err != nil {
		t.Fatalf("import first runtime snapshot: %v", err)
	}
	second, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a",
		SourceID:    "runs",
		SnapshotJSON: []byte(`{"runs":[
			{"run_id":"run-1","prompt":"Build","status":"completed","response":"done","updated_at":"2026-08-02T01:02:00Z"},
			{"run_id":"run-2","prompt":"Review","status":"accepted","updated_at":"2026-08-02T01:01:00Z"}
		]}`),
		ActorID: "runtime-sync",
	})
	if err != nil {
		t.Fatalf("import second runtime snapshot: %v", err)
	}
	if first.WorkIDs[0] == second.WorkIDs[0] {
		t.Fatal("changed run revision reused the previous work record")
	}

	raw, found, err := store.GetLegacyAgentRuntimeRunProjection(ctx, "workspace-a", "run-1")
	if err != nil || !found {
		t.Fatalf("get latest run projection found=%t err=%v", found, err)
	}
	var latest legacyRuntimeRun
	if err := json.Unmarshal(raw, &latest); err != nil {
		t.Fatalf("decode latest run projection: %v", err)
	}
	if latest.ID != "run-1" || latest.Status != "completed" || latest.Response != "done" {
		t.Fatalf("latest run projection = %#v", latest)
	}

	projections, err := store.ListLegacyAgentRuntimeRunProjections(ctx, "workspace-a", 10)
	if err != nil {
		t.Fatalf("list run projections: %v", err)
	}
	if len(projections) != 2 {
		t.Fatalf("run projection count = %d, want 2", len(projections))
	}
	var listed []legacyRuntimeRun
	for _, projection := range projections {
		var run legacyRuntimeRun
		if err := json.Unmarshal(projection, &run); err != nil {
			t.Fatalf("decode listed run projection: %v", err)
		}
		listed = append(listed, run)
	}
	if listed[0].ID != "run-2" || listed[1].ID != "run-1" {
		t.Fatalf("listed latest run order = %#v", listed)
	}
	if _, found, err := store.GetLegacyAgentRuntimeRunProjection(ctx, "workspace-b", "run-1"); err != nil || found {
		t.Fatalf("cross-workspace run projection found=%t err=%v", found, err)
	}
}

func TestAgentRuntimeImportReusesUnchangedRunsAcrossSnapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	firstRaw := json.RawMessage(`{"run_id":"run-stable","prompt":"Stable","status":"completed","updated_at":"2026-08-02T01:00:00Z"}`)
	first, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runs",
		SnapshotJSON: []byte(`{"runs":[` + string(firstRaw) + `]}`), ActorID: "runtime-sync",
	})
	if err != nil {
		t.Fatalf("import first snapshot: %v", err)
	}
	second, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runs",
		SnapshotJSON: []byte(`{"runs":[` + string(firstRaw) + `,{"run_id":"run-new","status":"accepted","updated_at":"2026-08-02T01:01:00Z"}]}`),
		ActorID:      "runtime-sync",
	})
	if err != nil {
		t.Fatalf("import expanded snapshot: %v", err)
	}
	if first.WorkIDs[0] != second.WorkIDs[0] {
		t.Fatalf("unchanged run work IDs = %q and %q", first.WorkIDs[0], second.WorkIDs[0])
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil || len(works) != 2 {
		t.Fatalf("work count after expanded snapshot = %d err=%v, want 2", len(works), err)
	}
}

func TestLegacyAgentRuntimeRunProjectionRejectsInvalidInputsAndMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	if _, err := store.ListLegacyAgentRuntimeRunProjections(ctx, "", 10); err == nil {
		t.Fatal("expected empty workspace list to fail")
	}
	if _, _, err := store.GetLegacyAgentRuntimeRunProjection(ctx, "workspace-a", ""); err == nil {
		t.Fatal("expected empty run id to fail")
	}
	result, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runs",
		SnapshotJSON: []byte(`{"runs":[{"run_id":"run-corrupt","status":"completed"}]}`),
		ActorID:      "runtime-sync",
	})
	if err != nil {
		t.Fatalf("import corruptible snapshot: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE works SET metadata_json = ? WHERE id = ?", []byte(`{"legacy_run":null}`), result.WorkIDs[0]); err != nil {
		t.Fatalf("corrupt runtime metadata: %v", err)
	}
	if _, _, err := store.GetLegacyAgentRuntimeRunProjection(ctx, "workspace-a", "run-corrupt"); err == nil {
		t.Fatal("expected missing legacy run metadata to fail")
	}
	if _, err := store.ListLegacyAgentRuntimeRunProjections(ctx, "workspace-a", 10); err == nil {
		t.Fatal("expected corrupt listed run metadata to fail")
	}
}

func TestImportRejectsCorruptOrIncompleteSourcesWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	if _, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: []byte("{"), TasksJSON: []byte(`{"tasks":[]}`), ActorID: "migration",
	}); err == nil {
		t.Fatal("corrupt session JSON was accepted")
	}
	if _, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: []byte(`{"title":"missing id"}`), TasksJSON: []byte(`{"tasks":[]}`), ActorID: "migration",
	}); err == nil {
		t.Fatal("session without an ID was accepted")
	}
	if _, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runtime", SnapshotJSON: []byte(`{"runs":[`), ActorID: "migration",
	}); err == nil {
		t.Fatal("corrupt runtime JSON was accepted")
	}
	if _, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runtime", SnapshotJSON: []byte(`{"runs":[{"status":"running"}]}`), ActorID: "migration",
	}); err == nil {
		t.Fatal("runtime run without an ID was accepted")
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list works: %v", err)
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("list markers: %v", err)
	}
	if len(works) != 0 || len(markers) != 0 {
		t.Fatalf("invalid imports mutated store: works=%d markers=%d", len(works), len(markers))
	}
}

func TestImportAgentRuntimeRejectsParentCycleBeforeMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	snapshot := []byte(`{"runs":[
		{"run_id":"run_root","status":"completed"},
		{"run_id":"run_a","parent_run_id":"run_b","status":"running"},
		{"run_id":"run_b","parent_run_id":"run_a","status":"accepted"}
	]}`)
	if _, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "cyclic", SnapshotJSON: snapshot, ActorID: "migration",
	}); err == nil {
		t.Fatal("cyclic runtime snapshot was accepted")
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list works: %v", err)
	}
	if len(works) != 0 {
		t.Fatalf("cyclic import left %d partial works, want 0", len(works))
	}
}

func TestImportAgentRuntimeLinksParentFromPriorSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	parentResult, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "first",
		SnapshotJSON: []byte(`{"runs":[{"run_id":"run_parent","agent":"builder","status":"completed"}]}`),
		ActorID:      "migration",
	})
	if err != nil {
		t.Fatalf("import parent snapshot: %v", err)
	}
	childResult, err := store.ImportAgentRuntimeSnapshot(ctx, AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "second",
		SnapshotJSON: []byte(`{"runs":[{"run_id":"run_child","parent_run_id":"run_parent","agent":"reviewer","status":"accepted"}]}`),
		ActorID:      "migration",
	})
	if err != nil {
		t.Fatalf("import child snapshot: %v", err)
	}
	child, err := store.GetWork(ctx, "workspace-a", childResult.WorkIDs[0])
	if err != nil {
		t.Fatalf("get child work: %v", err)
	}
	if child.ParentWorkID != parentResult.WorkIDs[0] || child.State != WorkStateReady {
		t.Fatalf("child parent/state = %q/%s, want %q/ready", child.ParentWorkID, child.State, parentResult.WorkIDs[0])
	}
}

func TestConcurrentLegacySessionImportCreatesOneSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	input := LegacySessionImportInput{
		WorkspaceID: "workspace-a",
		SessionJSON: []byte(`{"id":"session-concurrent","title":"Concurrent"}`),
		TasksJSON:   []byte(`{"tasks":[{"id":"task-1","title":"Task","status":"pending"}]}`),
		ActorID:     "migration",
	}
	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	workIDs := make(chan string, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.ImportLegacySession(ctx, input)
			if err == nil {
				workIDs <- result.WorkIDs[0]
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(workIDs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent import: %v", err)
		}
	}
	var expectedID string
	for workID := range workIDs {
		if expectedID == "" {
			expectedID = workID
		}
		if workID != expectedID {
			t.Fatalf("concurrent import work IDs = %q and %q", expectedID, workID)
		}
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list works: %v", err)
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("list markers: %v", err)
	}
	if len(works) != 1 || len(markers) != 1 {
		t.Fatalf("concurrent import counts = works:%d markers:%d, want 1/1", len(works), len(markers))
	}
}

func TestImportMappingsAndHelpers(t *testing.T) {
	t.Parallel()

	workStates := []struct {
		goal string
		plan string
		want WorkState
	}{
		{goal: "satisfied", want: WorkStateDone},
		{goal: "exhausted", want: WorkStateCancelled},
		{plan: "completed", want: WorkStateDone},
		{plan: "aborted", want: WorkStateCancelled},
		{plan: "paused", want: WorkStateBlocked},
		{plan: "executing", want: WorkStateRunning},
		{plan: "drafting", want: WorkStateTodo},
		{plan: "proposed", want: WorkStateTodo},
		{want: WorkStateBacklog},
	}
	for _, test := range workStates {
		if got := legacySessionWorkState(test.goal, test.plan); got != test.want {
			t.Errorf("legacy session state %q/%q = %s, want %s", test.goal, test.plan, got, test.want)
		}
	}
	runtimeStates := map[string]WorkState{
		"accepted": WorkStateReady, "running": WorkStateRunning,
		"completed": WorkStateDone, "succeeded": WorkStateDone,
		"failed": WorkStateBlocked, "cancelled": WorkStateCancelled,
		"canceled": WorkStateCancelled, "unknown": WorkStateTodo,
	}
	for status, want := range runtimeStates {
		if got := runtimeWorkState(status); got != want {
			t.Errorf("runtime work state %q = %s, want %s", status, got, want)
		}
	}
	attemptStates := map[string]AttemptStatus{
		"accepted": AttemptStatusPending, "pending": AttemptStatusPending,
		"running": AttemptStatusRunning, "completed": AttemptStatusSucceeded,
		"succeeded": AttemptStatusSucceeded, "failed": AttemptStatusFailed,
		"cancelled": AttemptStatusCancelled, "canceled": AttemptStatusCancelled,
		"unknown": AttemptStatusPending,
	}
	for status, want := range attemptStates {
		if got := runtimeAttemptStatus(status); got != want {
			t.Errorf("runtime attempt state %q = %s, want %s", status, got, want)
		}
	}
	if got := legacyEvidenceURI("session", "task", "evidence", legacyEvidence{URL: "https://example.com/proof"}); got != "https://example.com/proof" {
		t.Errorf("evidence URL = %q", got)
	}
	if got := legacyEvidenceURI("session", "task", "evidence", legacyEvidence{}); got != "legacy://session/session/task/task/evidence/evidence" {
		t.Errorf("fallback evidence URL = %q", got)
	}
	if got := parseImportTime("not-a-time"); got != nil {
		t.Errorf("invalid import time = %v, want nil", got)
	}
	if got := firstImportValue("", " value "); got != "value" {
		t.Errorf("first import value = %q", got)
	}
	if got := firstImportValue("", " "); got != "" {
		t.Errorf("empty first import value = %q", got)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return valuesEqual(leftValue, rightValue)
}

func valuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
