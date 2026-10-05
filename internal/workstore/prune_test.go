package workstore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func importRuns(t *testing.T, store *Store, snapshot string) ImportResult {
	t.Helper()
	result, err := store.ImportAgentRuntimeSnapshot(context.Background(), AgentRuntimeImportInput{
		WorkspaceID: "workspace-a", SourceID: "runs", SnapshotJSON: []byte(snapshot), ActorID: "sync",
	})
	if err != nil {
		t.Fatalf("import %s: %v", snapshot, err)
	}
	return result
}

func agentRunWorks(t *testing.T, store *Store) []Work {
	t.Helper()
	works, err := store.ListWorks(context.Background(), ListWorksFilter{WorkspaceID: "workspace-a", Source: string(ImportSourceAgentRuntime)})
	if err != nil {
		t.Fatalf("list agent-run works: %v", err)
	}
	return works
}

func TestImportKeepsOnlyTheNewestRevisionOfEachRun(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	ctx := context.Background()
	importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"running"},{"run_id":"run-2","prompt":"q","status":"running"}]}`)
	importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"running","response":"partial"},{"run_id":"run-2","prompt":"q","status":"running"}]}`)
	importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"completed","response":"done"},{"run_id":"run-2","prompt":"q","status":"running"}]}`)

	works := agentRunWorks(t, store)
	if len(works) != 2 {
		t.Fatalf("expected one work per run, got %d: %+v", len(works), works)
	}
	raw, found, err := store.GetLegacyAgentRuntimeRunProjection(ctx, "workspace-a", "run-1")
	if err != nil || !found {
		t.Fatalf("run-1 projection found=%v err=%v", found, err)
	}
	var run struct {
		Status   string `json:"status"`
		Response string `json:"response"`
	}
	if err := json.Unmarshal(raw, &run); err != nil || run.Status != "completed" || run.Response != "done" {
		t.Fatalf("run-1 should read as its newest revision, got %s err=%v", raw, err)
	}
	// Nothing of the pruned revisions is left behind.
	for _, table := range []string{"steps", "attempts"} {
		var rows int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil || rows != 2 {
			t.Fatalf("%s rows=%d err=%v, want 2", table, rows, err)
		}
	}
	var orphans int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE work_id NOT IN (SELECT id FROM works)").Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("orphan events=%d err=%v", orphans, err)
	}
	report, err := store.Doctor(ctx, "workspace-a")
	if err != nil || !report.Healthy {
		t.Fatalf("doctor after pruning: healthy=%v err=%v report=%+v", report.Healthy, err, report)
	}
	// Replaying the newest snapshot is still recognized.
	if again := importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"completed","response":"done"},{"run_id":"run-2","prompt":"q","status":"running"}]}`); !again.AlreadyImported {
		t.Fatal("expected the newest snapshot to be recognized as already imported")
	}
}

// A superseded revision that something else refers to is not plain history.
func TestPruneLeavesARevisionThatHasEvidence(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	ctx := context.Background()
	first := importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"running"}]}`)
	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "note", Source: "tester", IdempotencyKey: "child",
		ParentWorkID: first.WorkIDs[0], Title: "child of the first revision", ActorID: "tester",
	}); err != nil {
		t.Fatalf("create child: %v", err)
	}
	importRuns(t, store, `{"runs":[{"run_id":"run-1","prompt":"p","status":"completed"}]}`)

	if works := agentRunWorks(t, store); len(works) != 2 {
		t.Fatalf("expected the referenced revision kept next to the newest, got %d", len(works))
	}
	if _, err := store.PruneSupersededAgentRunRevisions(ctx, " "); err == nil {
		t.Fatal("expected an error without a workspace")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := store.PruneSupersededAgentRunRevisions(ctx, "workspace-a"); err == nil {
		t.Fatal("expected an error from a closed ledger")
	}
}
