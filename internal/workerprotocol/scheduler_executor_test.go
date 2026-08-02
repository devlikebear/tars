package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestSchedulerExecutorPersistsWorkspaceBeforeRemoteRunAndFinalizesAfterLedgerCommit(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileRemoteRunStore(filepath.Join(t.TempDir(), "remote-runs"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingRemoteCoordinator{runResult: RemoteRunResult{
		Succeeded: true,
		Payload:   json.RawMessage(`{"succeeded":true,"output":{"ok":true}}`),
	}}
	executor, err := NewSchedulerExecutor(SchedulerExecutorOptions{
		Adapter: "remote-preview", SourceDir: sourceDir, SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), Coordinator: coordinator, Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	execution := testRemoteSchedulerExecution("attempt-1")
	result, err := executor.Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("execute remote work: %v", err)
	}
	if !result.Succeeded || !bytes.Contains(result.OutputJSON, []byte(`"ok":true`)) {
		t.Fatalf("scheduler result=%+v", result)
	}
	if coordinator.runCalls != 1 || coordinator.lastRun.Workspace.Manifest.FileCount != 1 || coordinator.lastRun.Workspace.Files[0].Path != "main.go" {
		t.Fatalf("remote run input=%+v calls=%d", coordinator.lastRun, coordinator.runCalls)
	}
	if bytes.Contains(coordinator.lastRun.Request, []byte("contract-secret")) || bytes.Contains(coordinator.lastRun.Request, []byte("metadata-secret")) {
		t.Fatalf("remote request leaked ledger-only data: %s", coordinator.lastRun.Request)
	}
	state, found, err := store.Load(context.Background(), execution.Claim.Attempt.ID)
	if err != nil || !found || state.Result == nil || state.Input.Workspace.Manifest.Digest == "" {
		t.Fatalf("durable remote state=%+v found=%v err=%v", state, found, err)
	}

	if err := executor.Finalize(context.Background(), execution, result); err != nil {
		t.Fatalf("finalize remote journal: %v", err)
	}
	if coordinator.finalizeCalls != 1 {
		t.Fatalf("remote placement finalization calls=%d", coordinator.finalizeCalls)
	}
	if _, found, err := store.Load(context.Background(), execution.Claim.Attempt.ID); err != nil || found {
		t.Fatalf("remote journal survived ledger finalization found=%v err=%v", found, err)
	}
}

func TestSchedulerExecutorCommitsDurableRemoteResultWhenCleanupDisconnects(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileRemoteRunStore(filepath.Join(t.TempDir(), "remote-runs"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingRemoteCoordinator{
		runResult: RemoteRunResult{Succeeded: true, Payload: json.RawMessage(`{"succeeded":true,"durable":true}`)},
		runErr:    errors.New("disconnect after durable result"), resultStore: store,
	}
	executor, err := NewSchedulerExecutor(SchedulerExecutorOptions{
		Adapter: "remote-preview", SourceDir: sourceDir, SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), Coordinator: coordinator, Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	execution := testRemoteSchedulerExecution("attempt-cleanup-disconnect")
	result, err := executor.Execute(context.Background(), execution)
	if err != nil || !result.Succeeded || !bytes.Contains(result.OutputJSON, []byte(`"durable":true`)) {
		t.Fatalf("durable result after cleanup disconnect=%+v err=%v", result, err)
	}
	if _, found, err := store.Load(context.Background(), execution.Claim.Attempt.ID); err != nil || !found {
		t.Fatalf("recovery journal missing before ledger commit found=%v err=%v", found, err)
	}
	if err := executor.Finalize(context.Background(), execution, result); err != nil {
		t.Fatalf("retry cleanup after ledger commit: %v", err)
	}
	if coordinator.finalizeCalls != 1 {
		t.Fatalf("cleanup retry calls=%d", coordinator.finalizeCalls)
	}
	if _, found, err := store.Load(context.Background(), execution.Claim.Attempt.ID); err != nil || found {
		t.Fatalf("recovery journal survived successful cleanup found=%v err=%v", found, err)
	}
}

func TestSchedulerExecutorRecoversRecordedResultWithoutRepeatingRemoteExecution(t *testing.T) {
	t.Parallel()

	store, err := NewFileRemoteRunStore(filepath.Join(t.TempDir(), "remote-runs"))
	if err != nil {
		t.Fatal(err)
	}
	execution := testRemoteSchedulerExecution("attempt-recover")
	input := RemoteRunInput{
		PlacementID: "placement-attempt-recover", EnvironmentID: "environment-attempt-recover",
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID,
		StepID: execution.Claim.Step.ID, AttemptID: execution.Claim.Attempt.ID,
		Policy: DefaultExecutionPolicy(), Workspace: testRemoteWorkspaceBundle(),
		Request: json.RawMessage(`{"step":"Run checks"}`),
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	recorded := RemoteRunResult{Succeeded: true, Payload: json.RawMessage(`{"succeeded":true,"recovered":true}`)}
	if err := store.RecordResult(context.Background(), input, recorded); err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingRemoteCoordinator{}
	executor, err := NewSchedulerExecutor(SchedulerExecutorOptions{
		Adapter: "remote-preview", SourceDir: t.TempDir(), SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), Coordinator: coordinator, Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, found, err := executor.Recover(context.Background(), execution)
	if err != nil || !found || !result.Succeeded || coordinator.runCalls != 0 || coordinator.resumeCalls != 0 || coordinator.finalizeCalls != 1 {
		t.Fatalf("recover result=%+v found=%v err=%v coordinator=%+v", result, found, err, coordinator)
	}
}

func TestSchedulerExecutorRehydratesFromPersistedWorkspaceWhenResultIsMissing(t *testing.T) {
	t.Parallel()

	store, err := NewFileRemoteRunStore(filepath.Join(t.TempDir(), "remote-runs"))
	if err != nil {
		t.Fatal(err)
	}
	execution := testRemoteSchedulerExecution("attempt-resume")
	input := RemoteRunInput{
		PlacementID: "placement-attempt-resume", EnvironmentID: "environment-attempt-resume",
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID,
		StepID: execution.Claim.Step.ID, AttemptID: execution.Claim.Attempt.ID,
		Policy: DefaultExecutionPolicy(), Workspace: testRemoteWorkspaceBundle(),
		Request: json.RawMessage(`{"step":"original snapshot"}`),
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingRemoteCoordinator{resumeResult: RemoteRunResult{
		Succeeded: true, Payload: json.RawMessage(`{"succeeded":true,"resumed":true}`),
	}}
	executor, err := NewSchedulerExecutor(SchedulerExecutorOptions{
		Adapter: "remote-preview", SourceDir: t.TempDir(), SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), Coordinator: coordinator, Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, found, err := executor.Recover(context.Background(), execution)
	if err != nil || !found || !result.Succeeded || coordinator.resumeCalls != 1 {
		t.Fatalf("recover result=%+v found=%v err=%v coordinator=%+v", result, found, err, coordinator)
	}
	if coordinator.lastRecovery.Workspace.Manifest.Digest != input.Workspace.Manifest.Digest ||
		!bytes.Equal(coordinator.lastRecovery.Request, input.Request) {
		t.Fatalf("recovery did not use persisted input: %+v", coordinator.lastRecovery)
	}
}

func TestFileRemoteRunStoreRejectsConflictingResultAndPersistsNoTaskToken(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "remote-runs")
	store, err := NewFileRemoteRunStore(root)
	if err != nil {
		t.Fatal(err)
	}
	input := RemoteRunInput{
		PlacementID: "placement-a", EnvironmentID: "environment-a", WorkspaceID: "workspace-a",
		WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a", Policy: DefaultExecutionPolicy(),
		Workspace: testRemoteWorkspaceBundle(), Request: json.RawMessage(`{"objective":"safe"}`),
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	result := RemoteRunResult{Succeeded: true, Payload: json.RawMessage(`{"succeeded":true}`)}
	if err := store.RecordResult(context.Background(), input, result); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordResult(context.Background(), input, result); err != nil {
		t.Fatalf("idempotent result write: %v", err)
	}
	conflict := result
	conflict.Succeeded = false
	if err := store.RecordResult(context.Background(), input, conflict); err == nil {
		t.Fatal("conflicting remote result was accepted")
	}
	raw, err := os.ReadFile(filepath.Join(root, "attempt-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("task_token")) || bytes.Contains(raw, []byte("Bearer ")) {
		t.Fatalf("remote run journal persisted a credential: %s", raw)
	}
}

func testRemoteSchedulerExecution(attemptID string) workscheduler.Execution {
	return workscheduler.Execution{
		Work: workstore.Work{
			ID: "work-1", WorkspaceID: "workspace-1", Title: "Review the patch", Objective: "Check behavior",
			ContractJSON: json.RawMessage(`{"secret":"contract-secret"}`), MetadataJSON: json.RawMessage(`{"secret":"metadata-secret"}`),
		},
		Claim: workstore.StepClaim{
			Step:    workstore.Step{ID: "step-1", WorkID: "work-1", WorkspaceID: "workspace-1", Title: "Review", Description: "Run checks"},
			Attempt: workstore.Attempt{ID: attemptID},
		},
	}
}

func testRemoteWorkspaceBundle() WorkspaceBundle {
	raw := []byte("package main\n")
	digest := digestBytes(raw)
	manifest := WorkspaceManifest{
		SchemaVersion: workspaceManifestSchemaVersion, Mode: SyncModeDirectory,
		SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway,
		FileCount: 1, TotalBytes: int64(len(raw)),
		Entries:       []WorkspaceManifestEntry{{Path: "main.go", Digest: digest, SizeBytes: int64(len(raw)), Mode: 0o600}},
		ExcludedPaths: []string{},
	}
	manifest.Digest, _ = workspaceManifestDigest(manifest)
	return WorkspaceBundle{
		Manifest: manifest,
		Files:    []WorkspaceFile{{Path: "main.go", Digest: digest, Mode: 0o600, Data: raw}},
	}
}

type recordingRemoteCoordinator struct {
	runResult      RemoteRunResult
	resumeResult   RemoteRunResult
	runCalls       int
	resumeCalls    int
	finalizeCalls  int
	lastRun        RemoteRunInput
	lastRecovery   RemoteRecoveryInput
	lastFinalInput RemoteRunInput
	runErr         error
	resultStore    RemoteResultRecorder
}

func (coordinator *recordingRemoteCoordinator) Run(_ context.Context, input RemoteRunInput) (RemoteRunResult, error) {
	coordinator.runCalls++
	coordinator.lastRun = input
	if coordinator.resultStore != nil {
		if err := coordinator.resultStore.RecordResult(context.Background(), input, coordinator.runResult); err != nil {
			return RemoteRunResult{}, err
		}
	}
	return coordinator.runResult, coordinator.runErr
}

func (coordinator *recordingRemoteCoordinator) Resume(_ context.Context, input RemoteRecoveryInput) (RemoteRunResult, error) {
	coordinator.resumeCalls++
	coordinator.lastRecovery = input
	return coordinator.resumeResult, nil
}

func (coordinator *recordingRemoteCoordinator) RecoverPrepared(_ context.Context, input RemoteRunInput) (RemoteRunResult, error) {
	coordinator.resumeCalls++
	coordinator.lastRecovery = RemoteRecoveryInput{
		PlacementID: input.PlacementID, EnvironmentID: input.EnvironmentID,
		Workspace: cloneWorkspaceBundle(input.Workspace), Request: append(json.RawMessage(nil), input.Request...),
	}
	return coordinator.resumeResult, nil
}

func (coordinator *recordingRemoteCoordinator) FinalizeRecorded(_ context.Context, input RemoteRunInput, _ RemoteRunResult) error {
	coordinator.finalizeCalls++
	coordinator.lastFinalInput = input
	return nil
}
