package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestBootstrapWorkLedgerCanBeDisabledWithoutTouchingStorage(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	store, report, err := bootstrapWorkLedgerIfEnabled(context.Background(), false, workLedgerBootstrapOptions{
		WorkspaceDir: workspaceDir,
		Logger:       zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("disabled bootstrap: %v", err)
	}
	if store != nil {
		_ = store.Close()
		t.Fatal("disabled bootstrap returned a store")
	}
	if report.DatabasePath != "" {
		t.Fatalf("disabled bootstrap report = %+v, want zero value", report)
	}
	if _, err := os.Stat(workLedgerDatabasePath(workspaceDir)); !os.IsNotExist(err) {
		t.Fatalf("disabled bootstrap touched database path: %v", err)
	}
}

func TestBootstrapWorkLedgerImportsLegacySourcesIdempotently(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	sessionsDir := filepath.Join(workspaceDir, "sessions")
	runtimeDir := filepath.Join(workspaceDir, "_shared", "agentruntime")
	mustMkdirAll(t, sessionsDir)
	mustMkdirAll(t, runtimeDir)
	sessionIndex := []byte(`{
  "session-1": {
    "id": "session-1",
    "title": "Durable migration",
    "goal": {"description": "Finish safely", "status": "active"},
    "future_field": {"preserve": true},
    "created_at": "2026-08-01T00:00:00Z",
    "updated_at": "2026-08-01T01:00:00Z"
  }
}`)
	tasksJSON := []byte(`{
  "plan": {"goal": "Ship Phase 1", "status": "running"},
  "contract": {"status": "approved", "done_criteria": ["tests pass"]},
  "tasks": [{"id": "task-1", "title": "Wire bootstrap", "status": "in_progress"}]
}`)
	runsJSON := []byte(`{
  "runs": [
    {"run_id":"run-1","session_id":"session-1","agent":"planner","prompt":"plan","status":"completed","created_at":"2026-08-01T00:00:00Z","started_at":"2026-08-01T00:00:01Z","completed_at":"2026-08-01T00:00:02Z","updated_at":"2026-08-01T00:00:02Z"},
    {"run_id":"run-2","session_id":"session-1","parent_run_id":"run-1","agent":"worker","prompt":"build","status":"running","created_at":"2026-08-01T00:01:00Z","started_at":"2026-08-01T00:01:01Z","updated_at":"2026-08-01T00:01:02Z"}
  ]
}`)
	mustWriteFile(t, filepath.Join(sessionsDir, "sessions.json"), sessionIndex)
	mustWriteFile(t, filepath.Join(sessionsDir, "session-1.tasks.json"), tasksJSON)
	mustWriteFile(t, filepath.Join(runtimeDir, "runs.json"), runsJSON)
	opts := workLedgerBootstrapOptions{
		WorkspaceDir:               workspaceDir,
		AgentRuntimePersistenceDir: runtimeDir,
		Logger:                     zerolog.Nop(),
	}

	store, report, err := bootstrapWorkLedger(context.Background(), opts)
	if err != nil {
		t.Fatalf("bootstrap work ledger: %v", err)
	}
	if report.DatabasePath != workLedgerDatabasePath(workspaceDir) {
		t.Fatalf("database path = %q, want %q", report.DatabasePath, workLedgerDatabasePath(workspaceDir))
	}
	if report.LegacySessionsImported != 1 || report.AgentRuntimeWorksImported != 2 || report.QuarantinedSources != 0 {
		t.Fatalf("first bootstrap report = %#v", report)
	}
	if !report.Doctor.Healthy {
		t.Fatalf("first bootstrap doctor = %#v", report.Doctor)
	}
	works, err := store.ListWorks(context.Background(), workstore.ListWorksFilter{WorkspaceID: defaultWorkspaceID})
	if err != nil {
		t.Fatalf("list imported works: %v", err)
	}
	if len(works) != 3 {
		t.Fatalf("imported work count = %d, want 3", len(works))
	}
	projectedTasks, found, err := store.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, "session-1")
	if err != nil || !found {
		t.Fatalf("get legacy tasks projection found=%t err=%v", found, err)
	}
	if !handlerTestJSONEqual(projectedTasks, tasksJSON) {
		t.Fatalf("legacy tasks projection = %s, want %s", projectedTasks, tasksJSON)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first bootstrap store: %v", err)
	}

	reopened, replay, err := bootstrapWorkLedger(context.Background(), opts)
	if err != nil {
		t.Fatalf("replay work ledger bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if replay.LegacySessionsImported != 0 || replay.LegacySessionsReplayed != 1 || replay.AgentRuntimeWorksImported != 0 || !replay.AgentRuntimeSnapshotReplayed {
		t.Fatalf("replay bootstrap report = %#v", replay)
	}
	replayedWorks, err := reopened.ListWorks(context.Background(), workstore.ListWorksFilter{WorkspaceID: defaultWorkspaceID})
	if err != nil {
		t.Fatalf("list replayed works: %v", err)
	}
	if len(replayedWorks) != 3 {
		t.Fatalf("replayed work count = %d, want 3", len(replayedWorks))
	}
}

func TestBootstrapWorkLedgerQuarantinesMalformedSourcesWithoutDeletingThem(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	sessionsDir := filepath.Join(workspaceDir, "sessions")
	runtimeDir := filepath.Join(workspaceDir, "_shared", "agentruntime")
	mustMkdirAll(t, sessionsDir)
	mustMkdirAll(t, runtimeDir)
	indexJSON := []byte(`{"session-1":{"id":"session-1","title":"Recover valid session"}}`)
	brokenTasks := []byte(`{"tasks":[`)
	brokenRuns := []byte(`{"runs":[`)
	indexPath := filepath.Join(sessionsDir, "sessions.json")
	tasksPath := filepath.Join(sessionsDir, "session-1.tasks.json")
	runsPath := filepath.Join(runtimeDir, "runs.json")
	mustWriteFile(t, indexPath, indexJSON)
	mustWriteFile(t, tasksPath, brokenTasks)
	mustWriteFile(t, runsPath, brokenRuns)

	store, report, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{
		WorkspaceDir:               workspaceDir,
		AgentRuntimePersistenceDir: runtimeDir,
		Logger:                     zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("bootstrap with malformed sources: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if report.LegacySessionsImported != 1 || report.AgentRuntimeWorksImported != 0 || report.QuarantinedSources != 2 {
		t.Fatalf("malformed-source report = %#v", report)
	}
	if !report.Doctor.Healthy {
		t.Fatalf("doctor after quarantine = %#v", report.Doctor)
	}
	if got := mustReadFile(t, tasksPath); string(got) != string(brokenTasks) {
		t.Fatalf("tasks source changed: %q", got)
	}
	if got := mustReadFile(t, runsPath); string(got) != string(brokenRuns) {
		t.Fatalf("runtime source changed: %q", got)
	}
	projection, found, err := store.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, "session-1")
	if err != nil || !found || !handlerTestJSONEqual(projection, []byte(`{"tasks":[]}`)) {
		t.Fatalf("fallback legacy projection found=%t err=%v body=%s", found, err, projection)
	}
	entries, err := os.ReadDir(workLedgerQuarantineDir(workspaceDir))
	if err != nil {
		t.Fatalf("read quarantine directory: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("quarantine artifact count = %d, want 4", len(entries))
	}
	markers, err := store.ListImportMarkers(context.Background(), defaultWorkspaceID)
	if err != nil {
		t.Fatalf("list import markers: %v", err)
	}
	if len(markers) != 3 {
		t.Fatalf("import marker count = %d, want 3", len(markers))
	}
}

func TestBootstrapWorkLedgerQuarantinesMalformedSessionIndex(t *testing.T) {
	t.Parallel()

	for name, original := range map[string][]byte{
		"invalid-json": []byte(`{"session-1":`),
		"null-index":   []byte(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			workspaceDir := t.TempDir()
			indexPath := filepath.Join(workspaceDir, "sessions", "sessions.json")
			mustMkdirAll(t, filepath.Dir(indexPath))
			mustWriteFile(t, indexPath, original)

			store, report, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{
				WorkspaceDir: workspaceDir,
				Logger:       zerolog.Nop(),
			})
			if err != nil {
				t.Fatalf("bootstrap malformed session index: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if report.LegacySessionsImported != 0 || report.QuarantinedSources != 1 || !report.Doctor.Healthy {
				t.Fatalf("malformed-index report = %#v", report)
			}
			if got := mustReadFile(t, indexPath); string(got) != string(original) {
				t.Fatalf("session index changed: %q", got)
			}
		})
	}
}

func TestBootstrapWorkLedgerImportsMissingTasksAndQuarantinesUnsafeSessionID(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	indexPath := filepath.Join(workspaceDir, "sessions", "sessions.json")
	mustMkdirAll(t, filepath.Dir(indexPath))
	mustWriteFile(t, indexPath, []byte(`{
  "session-safe": {"id":"session-safe","title":"No task file"},
  "../escape": {"id":"../escape","title":"Unsafe"}
}`))

	store, report, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{
		WorkspaceDir: workspaceDir,
		Logger:       zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("bootstrap missing tasks and unsafe id: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if report.LegacySessionsImported != 1 || report.QuarantinedSources != 1 {
		t.Fatalf("bootstrap report = %#v", report)
	}
	projection, found, err := store.GetLegacySessionTasksProjection(context.Background(), defaultWorkspaceID, "session-safe")
	if err != nil || !found || !handlerTestJSONEqual(projection, []byte(`{"tasks":[]}`)) {
		t.Fatalf("missing-tasks projection found=%t err=%v body=%s", found, err, projection)
	}
}

func TestBootstrapWorkLedgerQuarantinesSemanticImportFailuresAndReplays(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	sessionsDir := filepath.Join(workspaceDir, "sessions")
	runtimeDir := filepath.Join(workspaceDir, "_shared", "agentruntime")
	mustMkdirAll(t, sessionsDir)
	mustMkdirAll(t, runtimeDir)
	mustWriteFile(t, filepath.Join(sessionsDir, "sessions.json"), []byte(`{"session-1":{"id":"session-1","title":"Bad tasks shape"}}`))
	mustWriteFile(t, filepath.Join(sessionsDir, "session-1.tasks.json"), []byte(`{"plan":"invalid","tasks":[]}`))
	mustWriteFile(t, filepath.Join(runtimeDir, "runs.json"), []byte(`{
  "runs": [
    {"run_id":"run-1","parent_run_id":"run-2","status":"running"},
    {"run_id":"run-2","parent_run_id":"run-1","status":"running"}
  ]
}`))
	opts := workLedgerBootstrapOptions{
		WorkspaceDir:               workspaceDir,
		AgentRuntimePersistenceDir: runtimeDir,
		Logger:                     zerolog.Nop(),
	}

	store, report, err := bootstrapWorkLedger(context.Background(), opts)
	if err != nil {
		t.Fatalf("bootstrap semantic import failures: %v", err)
	}
	if report.LegacySessionsImported != 0 || report.AgentRuntimeWorksImported != 0 || report.QuarantinedSources != 2 {
		t.Fatalf("semantic-failure report = %#v", report)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first semantic-failure store: %v", err)
	}

	reopened, replay, err := bootstrapWorkLedger(context.Background(), opts)
	if err != nil {
		t.Fatalf("replay semantic import failures: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if replay.QuarantinedSources != 0 || !replay.Doctor.Healthy {
		t.Fatalf("quarantine replay report = %#v", replay)
	}
	entries, err := os.ReadDir(workLedgerQuarantineDir(workspaceDir))
	if err != nil {
		t.Fatalf("read replayed quarantine directory: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("replayed quarantine artifact count = %d, want 4", len(entries))
	}
}

func TestBootstrapWorkLedgerRejectsInvalidRootsAndUnreadableSources(t *testing.T) {
	t.Parallel()

	if store, _, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{}); err == nil || store != nil {
		t.Fatalf("empty workspace bootstrap store=%v err=%v, want nil/error", store, err)
	}

	t.Run("workspace-is-file", func(t *testing.T) {
		workspacePath := filepath.Join(t.TempDir(), "workspace-file")
		mustWriteFile(t, workspacePath, []byte("not a directory"))
		if store, _, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{WorkspaceDir: workspacePath}); err == nil || store != nil {
			t.Fatalf("file workspace bootstrap store=%v err=%v, want nil/error", store, err)
		}
	})

	t.Run("session-index-is-directory", func(t *testing.T) {
		workspaceDir := t.TempDir()
		mustMkdirAll(t, filepath.Join(workspaceDir, "sessions", "sessions.json"))
		if store, _, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{WorkspaceDir: workspaceDir}); err == nil || store != nil {
			t.Fatalf("directory session index bootstrap store=%v err=%v, want nil/error", store, err)
		}
	})

	t.Run("runtime-snapshot-is-directory", func(t *testing.T) {
		workspaceDir := t.TempDir()
		runtimeDir := filepath.Join(workspaceDir, "runtime")
		mustMkdirAll(t, filepath.Join(runtimeDir, "runs.json"))
		if store, _, err := bootstrapWorkLedger(context.Background(), workLedgerBootstrapOptions{
			WorkspaceDir: workspaceDir, AgentRuntimePersistenceDir: runtimeDir,
		}); err == nil || store != nil {
			t.Fatalf("directory runtime snapshot bootstrap store=%v err=%v, want nil/error", store, err)
		}
	})
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return data
}
