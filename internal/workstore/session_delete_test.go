package workstore

import (
	"context"
	"path/filepath"
	"testing"
)

// A deleted session leaves nothing in the ledger, and the other sessions
// keep what they had.
func TestDeleteSessionRevisionsRemovesEveryRevisionOfThatSessionOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	evidence := `{"id":"ev_1","type":"test_result","title":"Verification: make test","command":"make test","status":"passed"}`
	tasks := []byte(`{"tasks":[{"id":"task-1","title":"First","status":"pending","evidence":[` + evidence + `]}]}`)
	importSession := func(id string) ImportResult {
		t.Helper()
		result, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
			WorkspaceID: "workspace-a",
			SessionJSON: []byte(`{"id":"` + id + `","title":"Session","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}`),
			TasksJSON:   tasks, SourcePath: "/legacy/" + id + ".json", ActorID: "migration",
		})
		if err != nil {
			t.Fatalf("import %s: %v", id, err)
		}
		return result
	}
	gone := importSession("session-gone")
	kept := importSession("session-kept")

	if _, err := store.DeleteSessionRevisions(ctx, "workspace-a", " "); err == nil {
		t.Fatal("a blank session id must be refused, not read as every session")
	}
	deleted, err := store.DeleteSessionRevisions(ctx, "workspace-a", "session-gone")
	if err != nil || deleted != 1 {
		t.Fatalf("deleted = %d err = %v, want 1", deleted, err)
	}

	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil || len(works) != 1 || works[0].ID != kept.WorkIDs[0] {
		t.Fatalf("works = %+v err = %v, want only the kept session's", works, err)
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil || len(markers) != 1 {
		t.Fatalf("markers = %d err = %v, want 1", len(markers), err)
	}
	for _, table := range []string{"steps", "proofs", "artifacts", "events"} {
		var orphans int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE work_id NOT IN (SELECT id FROM works)").Scan(&orphans); err != nil || orphans != 0 {
			t.Fatalf("%s orphans = %d err = %v", table, orphans, err)
		}
	}
	if _, found, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "session-gone"); err != nil || found {
		t.Fatalf("the deleted session is still served: found=%v err=%v", found, err)
	}
	if _, found, err := store.GetLegacySessionTasksProjection(ctx, "workspace-a", "session-kept"); err != nil || !found {
		t.Fatalf("the other session lost its record: found=%v err=%v", found, err)
	}
	if report, err := store.Doctor(ctx, "workspace-a"); err != nil || !report.Healthy {
		t.Fatalf("doctor after delete: healthy=%v err=%v", report.Healthy, err)
	}
	_ = gone
	if again, err := store.DeleteSessionRevisions(ctx, "workspace-a", "session-gone"); err != nil || again != 0 {
		t.Fatalf("a second delete = %d err = %v, want 0", again, err)
	}
}
