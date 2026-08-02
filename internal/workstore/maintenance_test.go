package workstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBackupAndRestorePreserveCommittedWALState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	store := openTestStore(t, filepath.Join(dir, "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "backup-work")
	step, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "backup-step",
		Title: "Persist me", State: WorkStateDone, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}

	backupPath := filepath.Join(dir, "backups", "ledger.sqlite")
	backup, err := store.Backup(ctx, backupPath)
	if err != nil {
		t.Fatalf("backup ledger: %v", err)
	}
	if backup.Path != backupPath || backup.Digest == "" || backup.SizeBytes <= 0 {
		t.Fatalf("backup report = %#v", backup)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %o, want 600", info.Mode().Perm())
	}
	if _, err := store.Backup(ctx, backupPath); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("overwrite backup error = %v, want ErrDestinationExists", err)
	}

	restorePath := filepath.Join(dir, "restored", "ledger.db")
	restored, err := RestoreBackup(ctx, backupPath, restorePath, Options{})
	if err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	if restored.SourcePath != backupPath || restored.TargetPath != restorePath || restored.Digest != backup.Digest {
		t.Fatalf("restore report = %#v, backup = %#v", restored, backup)
	}
	restoredStore, err := Open(ctx, restorePath, Options{})
	if err != nil {
		t.Fatalf("open restored ledger: %v", err)
	}
	t.Cleanup(func() { _ = restoredStore.Close() })
	if _, err := restoredStore.GetWork(ctx, work.WorkspaceID, work.ID); err != nil {
		t.Fatalf("get restored work: %v", err)
	}
	backupStore, err := Open(ctx, backupPath, Options{})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = backupStore.Close() })
	backupProjection, err := backupStore.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get work from backup: %v", err)
	}
	if len(backupProjection.Steps) != 1 || backupProjection.Steps[0].ID != step.ID {
		t.Fatalf("backup projection = %#v", backupProjection)
	}
	if _, err := RestoreBackup(ctx, backupPath, restorePath, Options{}); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("overwrite restore error = %v, want ErrDestinationExists", err)
	}

	corruptPath := filepath.Join(dir, "corrupt.sqlite")
	if err := os.WriteFile(corruptPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("write corrupt backup: %v", err)
	}
	corruptTarget := filepath.Join(dir, "corrupt-restored.db")
	if _, err := RestoreBackup(ctx, corruptPath, corruptTarget, Options{}); err == nil {
		t.Fatal("corrupt backup was restored")
	}
	if _, err := os.Stat(corruptTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt restore target exists: %v", err)
	}
}

func TestExportJSONLIsDeterministicAndChecksummed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	store := openTestStore(t, filepath.Join(dir, "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "export-work")
	first, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "first",
		Title: "First", State: WorkStateDone, Position: 1, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create first step: %v", err)
	}
	second, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "second",
		Title: "Second", State: WorkStateRunning, Position: 2, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		StepID: second.ID, DependsOnID: first.ID, ActorID: "tester",
	}); err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	attempt, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: second.ID,
		IdempotencyKey: "attempt", Number: 1, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	approval, err := store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: second.ID,
		AttemptID: attempt.ID, IdempotencyKey: "approval", Authority: "network",
		Status: ApprovalStatusPending, Request: "Allow", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	artifact, err := store.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: second.ID,
		AttemptID: attempt.ID, IdempotencyKey: "artifact", Kind: "log",
		Name: "test.log", URI: "file:///test.log", Digest: "sha256:1234", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: second.ID,
		AttemptID: attempt.ID, IdempotencyKey: "proof", Kind: "test",
		Status: ProofStatusPassed, Summary: "passed", ArtifactID: artifact.ID, ActorID: "tester",
	}); err != nil {
		t.Fatalf("create proof: %v", err)
	}
	if approval.ID == "" {
		t.Fatal("approval was not created")
	}
	if _, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: []byte(`{"id":"session-export","title":"Imported"}`),
		TasksJSON: []byte(`{"tasks":[]}`), ActorID: "migration",
	}); err != nil {
		t.Fatalf("create import marker: %v", err)
	}

	firstPath := filepath.Join(dir, "export-1.jsonl")
	secondPath := filepath.Join(dir, "export-2.jsonl")
	firstExport, err := store.ExportJSONL(ctx, "workspace-a", firstPath)
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	secondExport, err := store.ExportJSONL(ctx, "workspace-a", secondPath)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if firstExport.Digest != secondExport.Digest || firstExport.RecordCount != secondExport.RecordCount || firstExport.RecordCount < 10 {
		t.Fatalf("exports differ: first=%#v second=%#v", firstExport, secondExport)
	}
	firstRaw, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	secondRaw, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second export: %v", err)
	}
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("deterministic exports have different bytes")
	}
	footerStart := bytes.LastIndex(firstRaw, []byte(`{"type":"checksum"`))
	if footerStart <= 0 {
		t.Fatalf("checksum footer missing from export: %s", firstRaw)
	}
	sum := sha256.Sum256(firstRaw[:footerStart])
	if got := hex.EncodeToString(sum[:]); got != firstExport.Digest {
		t.Fatalf("export digest = %q, recomputed %q", firstExport.Digest, got)
	}
	types := make(map[string]bool)
	for _, line := range bytes.Split(bytes.TrimSpace(firstRaw), []byte("\n")) {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatalf("decode JSONL line %q: %v", line, err)
		}
		types[envelope.Type] = true
	}
	for _, recordType := range []string{"header", "work", "step", "dependency", "attempt", "event", "approval", "artifact", "proof", "import_marker", "checksum"} {
		if !types[recordType] {
			t.Errorf("export missing %q record", recordType)
		}
	}
	if _, err := store.ExportJSONL(ctx, "workspace-a", firstPath); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("overwrite export error = %v, want ErrDestinationExists", err)
	}
}

func TestDoctorReportsHealthyLedgerAndInvalidJSON(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "doctor-work")
	report, err := store.Doctor(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("doctor healthy ledger: %v", err)
	}
	if !report.Healthy || len(report.Checks) < 4 || len(report.Issues) != 0 {
		t.Fatalf("healthy doctor report = %#v", report)
	}

	if _, err := store.db.ExecContext(ctx, "UPDATE works SET metadata_json = '{' WHERE id = ?", work.ID); err != nil {
		t.Fatalf("corrupt work metadata: %v", err)
	}
	report, err = store.Doctor(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("doctor corrupt ledger: %v", err)
	}
	if report.Healthy || !hasDoctorIssue(report, "invalid_json", "work", work.ID) {
		t.Fatalf("corrupt doctor report = %#v", report)
	}
}

func TestDoctorReportsTerminalDependencyImportAndMigrationIssues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "doctor-invariants")
	first, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "first",
		Title: "First", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create first step: %v", err)
	}
	second, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "second",
		Title: "Second", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		StepID: second.ID, DependsOnID: first.ID, ActorID: "tester",
	}); err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	imported, err := store.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: []byte(`{"id":"doctor-import","title":"Import"}`),
		TasksJSON: []byte(`{"tasks":[]}`), ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("create import marker: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, "UPDATE works SET state = 'done', completed_at = NULL WHERE id = ?", work.ID); err != nil {
		t.Fatalf("remove terminal timestamp: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO step_dependencies (workspace_id, work_id, step_id, depends_on_step_id, actor_id, created_at)
		VALUES (?, ?, ?, ?, 'tamper', 0)
	`, work.WorkspaceID, work.ID, first.ID, second.ID); err != nil {
		t.Fatalf("insert dependency cycle: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE import_markers SET work_ids_json = '[\"wrk_missing\"]' WHERE id = ?", imported.Marker.ID); err != nil {
		t.Fatalf("break import reference: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE schema_migrations SET checksum = 'tampered' WHERE version = ?", schemaVersion); err != nil {
		t.Fatalf("tamper migration: %v", err)
	}

	report, err := store.Doctor(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("doctor invalid ledger: %v", err)
	}
	if report.Healthy {
		t.Fatalf("doctor marked invalid ledger healthy: %#v", report)
	}
	for _, expected := range []struct {
		code       string
		recordType string
		recordID   string
	}{
		{"missing_terminal_timestamp", "work", work.ID},
		{"dependency_cycle", "step", first.ID},
		{"missing_import_work", "import_marker", imported.Marker.ID},
	} {
		if !hasDoctorIssue(report, expected.code, expected.recordType, expected.recordID) {
			t.Errorf("doctor report missing %s for %s %s: %#v", expected.code, expected.recordType, expected.recordID, report)
		}
	}
	if doctorCheckOK(report, "migrations") {
		t.Fatalf("migration check unexpectedly passed: %#v", report.Checks)
	}
}

func TestQuarantineSourceCopiesWithoutDeletingOriginalAndIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	store := openTestStore(t, filepath.Join(dir, "ledger.db"))
	sourcePath := filepath.Join(dir, "broken-session.json")
	source := []byte(`{"id":"broken"`)
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatalf("write corrupt source: %v", err)
	}
	input := QuarantineInput{
		WorkspaceID:   "workspace-a",
		SourceKind:    ImportSourceLegacySession,
		SourceID:      "broken-session",
		SourcePath:    sourcePath,
		QuarantineDir: filepath.Join(dir, "quarantine"),
		Reason:        "invalid JSON",
		ActorID:       "doctor",
	}
	record, err := store.QuarantineSource(ctx, input)
	if err != nil {
		t.Fatalf("quarantine source: %v", err)
	}
	if record.AlreadyQuarantined || record.Digest == "" || record.Marker.Status != ImportStatusQuarantined {
		t.Fatalf("quarantine record = %#v", record)
	}
	original, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read original after quarantine: %v", err)
	}
	copyRaw, err := os.ReadFile(record.QuarantinePath)
	if err != nil {
		t.Fatalf("read quarantine copy: %v", err)
	}
	if !bytes.Equal(original, source) || !bytes.Equal(copyRaw, source) {
		t.Fatal("quarantine did not preserve exact source bytes")
	}
	info, err := os.Stat(record.QuarantinePath)
	if err != nil {
		t.Fatalf("stat quarantine copy: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("quarantine permissions = %o, want 600", info.Mode().Perm())
	}
	manifestRaw, err := os.ReadFile(record.ManifestPath)
	if err != nil {
		t.Fatalf("read quarantine manifest: %v", err)
	}
	var manifest QuarantineRecord
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode quarantine manifest: %v", err)
	}
	if manifest.SourcePath != sourcePath || manifest.Digest != record.Digest || manifest.Reason != input.Reason {
		t.Fatalf("quarantine manifest = %#v", manifest)
	}

	replayed, err := store.QuarantineSource(ctx, input)
	if err != nil {
		t.Fatalf("replay quarantine: %v", err)
	}
	if !replayed.AlreadyQuarantined || replayed.Marker.ID != record.Marker.ID || replayed.QuarantinePath != record.QuarantinePath {
		t.Fatalf("replayed quarantine = %#v, first = %#v", replayed, record)
	}
	markers, err := store.ListImportMarkers(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("list quarantine markers: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("quarantine marker count = %d, want 1", len(markers))
	}
}

func TestQuarantineSourceReplayAcrossReopenKeepsCanonicalManifest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "ledger.db")
	sourcePath := filepath.Join(dir, "broken-runtime.json")
	if err := os.WriteFile(sourcePath, []byte(`{"runs":[`), 0o600); err != nil {
		t.Fatalf("write corrupt source: %v", err)
	}
	firstStore, err := Open(ctx, databasePath, Options{})
	if err != nil {
		t.Fatalf("open first quarantine store: %v", err)
	}
	first, err := firstStore.QuarantineSource(ctx, QuarantineInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceAgentRuntime,
		SourceID: "runs", SourcePath: sourcePath, QuarantineDir: filepath.Join(dir, "quarantine"),
		Reason: "first decode failure", ActorID: "bootstrap-v1",
	})
	if err != nil {
		t.Fatalf("first quarantine: %v", err)
	}
	manifestBefore, err := os.ReadFile(first.ManifestPath)
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first quarantine store: %v", err)
	}

	reopened, err := Open(ctx, databasePath, Options{})
	if err != nil {
		t.Fatalf("reopen quarantine store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, err := reopened.QuarantineSource(ctx, QuarantineInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceAgentRuntime,
		SourceID: "runs", SourcePath: sourcePath, QuarantineDir: filepath.Join(dir, "quarantine"),
		Reason: "retry reported a more detailed decode failure", ActorID: "bootstrap-v2",
	})
	if err != nil {
		t.Fatalf("replay quarantine after reopen: %v", err)
	}
	if !replayed.AlreadyQuarantined || replayed.Marker.ID != first.Marker.ID || replayed.ManifestPath != first.ManifestPath {
		t.Fatalf("replayed quarantine = %#v, first = %#v", replayed, first)
	}
	if replayed.Reason != first.Reason || replayed.ActorID != first.ActorID {
		t.Fatalf("replay did not keep canonical provenance: %#v", replayed)
	}
	manifestAfter, err := os.ReadFile(replayed.ManifestPath)
	if err != nil {
		t.Fatalf("read replayed manifest: %v", err)
	}
	if !bytes.Equal(manifestAfter, manifestBefore) {
		t.Fatalf("replay changed canonical manifest:\nbefore=%s\nafter=%s", manifestBefore, manifestAfter)
	}
}

func TestMaintenanceValidationAndFilesystemFailurePaths(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := openTestStore(t, filepath.Join(dir, "ledger.db"))

	if _, err := store.Backup(ctx, ""); err == nil {
		t.Fatal("empty backup destination was accepted")
	}
	if _, err := RestoreBackup(ctx, "", "", Options{}); err == nil {
		t.Fatal("empty restore paths were accepted")
	}
	if _, err := store.ExportJSONL(ctx, "", ""); err == nil {
		t.Fatal("empty export input was accepted")
	}
	if _, err := store.Doctor(ctx, ""); err == nil {
		t.Fatal("empty doctor workspace was accepted")
	}
	if _, err := store.QuarantineSource(ctx, QuarantineInput{}); err == nil {
		t.Fatal("empty quarantine input was accepted")
	}
	if _, err := store.QuarantineSource(ctx, QuarantineInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceLegacySession,
		SourceID: "missing", SourcePath: filepath.Join(dir, "missing.json"),
		QuarantineDir: filepath.Join(dir, "quarantine"), Reason: "missing", ActorID: "doctor",
	}); err == nil {
		t.Fatal("missing quarantine source was accepted")
	}

	backupPath := filepath.Join(dir, "backup.sqlite")
	if _, err := store.Backup(ctx, backupPath); err != nil {
		t.Fatalf("create validation backup: %v", err)
	}
	if _, err := RestoreBackup(ctx, backupPath, backupPath, Options{}); err == nil {
		t.Fatal("same restore source and target was accepted")
	}
	if _, err := RestoreBackup(ctx, filepath.Join(dir, "missing.sqlite"), filepath.Join(dir, "target.sqlite"), Options{}); err == nil {
		t.Fatal("missing restore source was accepted")
	}
	blockedParent := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	if _, err := store.Backup(ctx, filepath.Join(blockedParent, "backup.sqlite")); err == nil {
		t.Fatal("backup under a file was accepted")
	}
	if _, err := store.ExportJSONL(ctx, "workspace-a", filepath.Join(blockedParent, "export.jsonl")); err == nil {
		t.Fatal("export under a file was accepted")
	}
	if _, err := RestoreBackup(ctx, backupPath, filepath.Join(blockedParent, "restore.sqlite"), Options{}); err == nil {
		t.Fatal("restore under a file was accepted")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Backup(canceled, filepath.Join(dir, "canceled-backup.sqlite")); err == nil {
		t.Fatal("backup with canceled context succeeded")
	}
	if _, err := store.ExportJSONL(canceled, "workspace-a", filepath.Join(dir, "canceled-export.jsonl")); err == nil {
		t.Fatal("export with canceled context succeeded")
	}

	existing := filepath.Join(dir, "existing.bin")
	if err := os.WriteFile(existing, []byte("same"), 0o600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	if err := requireMissingDestination(existing); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("existing destination error = %v", err)
	}
	if err := writeExactFile(existing, []byte("same"), 0o600); err != nil {
		t.Fatalf("idempotent exact write: %v", err)
	}
	if err := writeExactFile(existing, []byte("different"), 0o600); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("different exact write error = %v", err)
	}
	if err := writeExactFile(filepath.Join(dir, "missing-write-dir", "file"), []byte("value"), 0o600); err == nil {
		t.Fatal("exact write under a missing directory succeeded")
	}
	if _, err := vacantTemporaryPath(filepath.Join(dir, "missing-dir"), "temp-*"); err == nil {
		t.Fatal("temporary path in missing directory succeeded")
	}
	if _, err := copyToTemporaryFile(filepath.Join(dir, "missing-source"), dir, "copy-*"); err == nil {
		t.Fatal("copy of missing source succeeded")
	}
	if _, err := copyToTemporaryFile(existing, filepath.Join(dir, "missing-dir"), "copy-*"); err == nil {
		t.Fatal("copy into missing directory succeeded")
	}
	if _, _, err := fileDigest(filepath.Join(dir, "missing-digest")); err == nil {
		t.Fatal("digest of missing file succeeded")
	}
	if err := syncDirectory(filepath.Join(dir, "missing-sync-dir")); err == nil {
		t.Fatal("sync of missing directory succeeded")
	}
	if bytesEqual([]byte("a"), []byte("ab")) || bytesEqual([]byte("a"), []byte("b")) || !bytesEqual([]byte("a"), []byte("a")) {
		t.Fatal("byte equality helper returned an invalid result")
	}

	temporary := filepath.Join(dir, "temporary")
	if err := os.WriteFile(temporary, []byte("temp"), 0o600); err != nil {
		t.Fatalf("write publish temporary: %v", err)
	}
	if err := publishTemporaryFile(temporary, existing); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("publish collision error = %v, want ErrDestinationExists", err)
	}
	removeFile(temporary)
	if err := publishTemporaryFile(filepath.Join(dir, "missing-publish"), filepath.Join(dir, "published")); err == nil {
		t.Fatal("publish of missing temporary succeeded")
	}

	closed := openTestStore(t, filepath.Join(dir, "closed.db"))
	if err := closed.Close(); err != nil {
		t.Fatalf("close diagnostic store: %v", err)
	}
	if _, err := closed.Backup(ctx, filepath.Join(dir, "closed-backup.sqlite")); err == nil {
		t.Fatal("backup of closed store succeeded")
	}
	report, err := closed.Doctor(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("doctor closed store returned a fatal error: %v", err)
	}
	if report.Healthy {
		t.Fatalf("doctor marked closed store healthy: %#v", report)
	}
	canceledMarkerContext, cancelMarker := context.WithCancel(ctx)
	cancelMarker()
	if _, _, err := store.recordImportMarker(canceledMarkerContext, importMarkerInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceLegacySession,
		SourceID: "canceled", Checksum: "checksum", Status: ImportStatusFailed,
		ActorID: "doctor", ErrorText: "canceled",
	}); err == nil {
		t.Fatal("import marker with canceled context succeeded")
	}
}

func TestDoctorReportsAllTerminalRecordTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "doctor-terminal-types")
	step, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "step",
		Title: "Step", ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "attempt", Number: 1, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE steps SET state = 'cancelled', completed_at = NULL WHERE id = ?", step.ID); err != nil {
		t.Fatalf("corrupt terminal step: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE attempts SET status = 'failed', finished_at = NULL WHERE id = ?", attempt.ID); err != nil {
		t.Fatalf("corrupt terminal attempt: %v", err)
	}
	report, err := store.Doctor(ctx, work.WorkspaceID)
	if err != nil {
		t.Fatalf("doctor terminal types: %v", err)
	}
	if !hasDoctorIssue(report, "missing_terminal_timestamp", "step", step.ID) ||
		!hasDoctorIssue(report, "missing_terminal_timestamp", "attempt", attempt.ID) {
		t.Fatalf("doctor terminal issues = %#v", report.Issues)
	}
}

func TestJSONLWriterAndMigrationFailurePaths(t *testing.T) {
	t.Parallel()

	writer := bufio.NewWriterSize(errorWriter{}, 1)
	if err := writeHashedJSONLine(writer, sha256.New(), map[string]string{"value": "long enough to write"}); err == nil {
		t.Fatal("hashed JSONL write to failing writer succeeded")
	}
	var output bytes.Buffer
	writer = bufio.NewWriter(&output)
	if err := writeHashedJSONLine(writer, errorHash{Hash: sha256.New()}, map[string]string{"value": "ok"}); err == nil {
		t.Fatal("hashed JSONL write with failing digest succeeded")
	}
	if err := writeHashedJSONLine(writer, sha256.New(), make(chan int)); err == nil {
		t.Fatal("hashed JSONL marshaled an unsupported value")
	}
	writer = bufio.NewWriterSize(errorWriter{}, 1)
	if err := writeJSONLine(writer, map[string]string{"value": "long enough to write"}); err == nil {
		t.Fatal("JSONL footer write to failing writer succeeded")
	}
	writer = bufio.NewWriter(io.Discard)
	if err := writeJSONLine(writer, make(chan int)); err == nil {
		t.Fatal("JSONL footer marshaled an unsupported value")
	}

	writer = bufio.NewWriter(io.Discard)
	exporter := newJSONLExporter(writer)
	if err := exporter.writeHeader("workspace-a", time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("write exporter header: %v", err)
	}
	if err := exporter.writeProjection(WorkProjection{Work: Work{MetadataJSON: json.RawMessage("{")}}); err == nil {
		t.Fatal("exporter accepted invalid raw JSON")
	}
	exporter = newJSONLExporter(bufio.NewWriter(io.Discard))
	if err := exporter.writeProjection(WorkProjection{
		Work:     Work{ID: "work"},
		Attempts: []Attempt{{ID: "attempt", InputJSON: json.RawMessage("{")}},
	}); err == nil {
		t.Fatal("exporter accepted an invalid nested record")
	}
	exporter = &jsonlExporter{writer: bufio.NewWriter(io.Discard), digest: errorHash{Hash: sha256.New()}, recordCounts: make(map[string]int)}
	if err := writeExportRecords(exporter, "work", []Work{{ID: "work"}}); err == nil {
		t.Fatal("record collection write with failing digest succeeded")
	}

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	if _, err := store.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", schemaVersion); err != nil {
		t.Fatalf("delete migration: %v", err)
	}
	if err := verifyMigrationsDB(ctx, store.db, true); err == nil {
		t.Fatal("required migration gap was accepted")
	}
	if err := verifyMigrationsDB(ctx, store.db, false); err != nil {
		t.Fatalf("optional future migration gap was rejected: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO schema_migrations (version, checksum, applied_at) VALUES (99, 'future', 0)"); err != nil {
		t.Fatalf("insert unsupported migration: %v", err)
	}
	if err := verifyMigrationsDB(ctx, store.db, false); err == nil {
		t.Fatal("unsupported future migration was accepted")
	}
}

func TestQuarantineRejectsBlockedDirectoryCollisionAndMarkerFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "broken.json")
	raw := []byte("broken")
	if err := os.WriteFile(sourcePath, raw, 0o600); err != nil {
		t.Fatalf("write quarantine source: %v", err)
	}
	blockedDir := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blockedDir, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocked quarantine directory: %v", err)
	}
	store := openTestStore(t, filepath.Join(dir, "ledger.db"))
	baseInput := QuarantineInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceLegacySession,
		SourceID: "broken", SourcePath: sourcePath, Reason: "broken", ActorID: "doctor",
	}
	blockedInput := baseInput
	blockedInput.QuarantineDir = blockedDir
	if _, err := store.QuarantineSource(ctx, blockedInput); err == nil {
		t.Fatal("quarantine into a file succeeded")
	}

	quarantineDir := filepath.Join(dir, "collision")
	if err := os.MkdirAll(quarantineDir, 0o700); err != nil {
		t.Fatalf("create collision directory: %v", err)
	}
	digest := digestBytes(raw)
	copyPath, _ := quarantinePaths(quarantineDir, sourcePath, digest, store.now().UTC())
	if err := os.WriteFile(copyPath, []byte("different"), 0o600); err != nil {
		t.Fatalf("write colliding quarantine file: %v", err)
	}
	collisionInput := baseInput
	collisionInput.QuarantineDir = quarantineDir
	if _, err := store.QuarantineSource(ctx, collisionInput); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("quarantine collision error = %v, want ErrDestinationExists", err)
	}

	failureDir := filepath.Join(dir, "marker-failure")
	failureInput := baseInput
	failureInput.SourceID = "marker-failure"
	failureInput.QuarantineDir = failureDir
	store.newID = func(string) (string, error) { return "", errors.New("id unavailable") }
	if _, err := store.QuarantineSource(ctx, failureInput); err == nil || !bytes.Contains([]byte(err.Error()), []byte("id unavailable")) {
		t.Fatalf("marker ID failure error = %v", err)
	}
	if base, manifest := quarantinePaths(failureDir, sourcePath, digest, store.now().UTC()); manifest == base || base == "" {
		t.Fatalf("unexpected quarantine paths %q/%q", base, manifest)
	}
	if base, _ := quarantinePaths(failureDir, ".", digest, store.now().UTC()); filepath.Base(base) == "." {
		t.Fatalf("quarantine path did not sanitize dot basename: %q", base)
	}
}

func TestConcurrentQuarantineConvergesOnOneCopyAndMarker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	var mu sync.Mutex
	var timeSequence, idSequence int
	baseTime := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	store, err := Open(ctx, filepath.Join(dir, "ledger.db"), Options{
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			timeSequence++
			return baseTime.Add(time.Duration(timeSequence) * time.Millisecond)
		},
		NewID: func(prefix string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			idSequence++
			return fmt.Sprintf("%s_%04d", prefix, idSequence), nil
		},
	})
	if err != nil {
		t.Fatalf("open quarantine store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sourcePath := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(sourcePath, []byte("broken"), 0o600); err != nil {
		t.Fatalf("write quarantine source: %v", err)
	}
	input := QuarantineInput{
		WorkspaceID: "workspace-a", SourceKind: ImportSourceLegacySession,
		SourceID: "broken", SourcePath: sourcePath,
		QuarantineDir: filepath.Join(dir, "quarantine"),
		Reason:        "invalid JSON", ActorID: "doctor",
	}
	const callers = 8
	var wg sync.WaitGroup
	records := make(chan QuarantineRecord, callers)
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := store.QuarantineSource(ctx, input)
			records <- record
			errs <- err
		}()
	}
	wg.Wait()
	close(records)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent quarantine: %v", err)
		}
	}
	var markerID, copyPath string
	for record := range records {
		if markerID == "" {
			markerID, copyPath = record.Marker.ID, record.QuarantinePath
		}
		if record.Marker.ID != markerID || record.QuarantinePath != copyPath {
			t.Fatalf("quarantine did not converge: first=%s/%s next=%s/%s", markerID, copyPath, record.Marker.ID, record.QuarantinePath)
		}
	}
	entries, err := os.ReadDir(input.QuarantineDir)
	if err != nil {
		t.Fatalf("read quarantine directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("quarantine file count = %d, want copy plus manifest", len(entries))
	}
	markers, err := store.ListImportMarkers(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatalf("list quarantine markers: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("quarantine marker count = %d, want 1", len(markers))
	}
}

func TestExportRejectsCorruptProjectionAndImportMarker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	projectionStore := openTestStore(t, filepath.Join(dir, "projection.db"))
	work := mustCreateWork(t, projectionStore, "workspace-a", "corrupt-export")
	if _, err := projectionStore.db.ExecContext(ctx, "UPDATE works SET metadata_json = '{' WHERE id = ?", work.ID); err != nil {
		t.Fatalf("corrupt export projection: %v", err)
	}
	if _, err := projectionStore.ExportJSONL(ctx, "workspace-a", filepath.Join(dir, "projection.jsonl")); err == nil {
		t.Fatal("export of invalid projection JSON succeeded")
	}

	markerStore := openTestStore(t, filepath.Join(dir, "marker.db"))
	imported, err := markerStore.ImportLegacySession(ctx, LegacySessionImportInput{
		WorkspaceID: "workspace-a", SessionJSON: []byte(`{"id":"marker-export","title":"Marker"}`),
		TasksJSON: []byte(`{"tasks":[]}`), ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("create marker fixture: %v", err)
	}
	if _, err := markerStore.db.ExecContext(ctx, "UPDATE import_markers SET work_ids_json = '{' WHERE id = ?", imported.Marker.ID); err != nil {
		t.Fatalf("corrupt import marker: %v", err)
	}
	if _, err := markerStore.ExportJSONL(ctx, "workspace-a", filepath.Join(dir, "marker.jsonl")); err == nil {
		t.Fatal("export of invalid import marker JSON succeeded")
	}

	scanStore := openTestStore(t, filepath.Join(dir, "scan.db"))
	scanWork := mustCreateWork(t, scanStore, "workspace-a", "scan-export")
	if _, err := scanStore.db.ExecContext(ctx, "UPDATE works SET contract_json = 42 WHERE id = ?", scanWork.ID); err != nil {
		t.Fatalf("corrupt projection storage type: %v", err)
	}
	if _, err := scanStore.ExportJSONL(ctx, "workspace-a", filepath.Join(dir, "scan.jsonl")); err == nil {
		t.Fatal("export of unscannable projection succeeded")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected writer failure")
}

type errorHash struct {
	hash.Hash
}

func (errorHash) Write([]byte) (int, error) {
	return 0, errors.New("injected hash failure")
}

func hasDoctorIssue(report DoctorReport, code, recordType, recordID string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code && issue.RecordType == recordType && issue.RecordID == recordID {
			return true
		}
	}
	return false
}

func doctorCheckOK(report DoctorReport, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return check.OK
		}
	}
	return false
}
