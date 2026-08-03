package workerprotocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGatewayCoordinatorFinalizesDurablyRecordedResultAfterRestartWithoutReexecution(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 51
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingReferenceExecutor{result: ReferenceExecutionResult{
		Payload: json.RawMessage(`{"succeeded":true,"summary":"recorded before crash"}`),
	}}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewInProcessTransport(worker, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(t.TempDir(), "controller.json")
	controller, err := OpenController(ControllerOptions{StatePath: controllerPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewFileRemoteRunStore(filepath.Join(t.TempDir(), "remote-runs"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordThenFailRemoteResult{store: store, err: errors.New("simulated gateway crash")}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		ResultRecorder: recorder, LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	input := RemoteRunInput{
		PlacementID: "placement-crash", EnvironmentID: "environment-crash",
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-crash",
		Policy: DefaultExecutionPolicy(), Workspace: bundle, Request: json.RawMessage(`{"objective":"run once"}`),
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Run(context.Background(), input); !errors.Is(err, recorder.err) {
		t.Fatalf("run error=%v want simulated crash", err)
	}
	state, found, err := store.Load(context.Background(), input.AttemptID)
	if err != nil || !found || state.Result == nil {
		t.Fatalf("durable result after crash state=%+v found=%v err=%v", state, found, err)
	}
	if placement := controller.Snapshot().Placements[input.PlacementID]; placement.State != PlacementStateCollecting {
		t.Fatalf("placement before recovery=%+v", placement)
	}

	reopened, err := OpenController(ControllerOptions{StatePath: controllerPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: reopened, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		ResultRecorder: store, LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.FinalizeRecorded(context.Background(), state.Input, *state.Result); err != nil {
		t.Fatalf("finalize recorded result after restart: %v", err)
	}
	if placement := reopened.Snapshot().Placements[input.PlacementID]; placement.State != PlacementStateDestroyed {
		t.Fatalf("placement after recovery=%+v", placement)
	}
	if executor.calls != 1 {
		t.Fatalf("remote execution repeated after recovery: calls=%d", executor.calls)
	}
}

func TestGatewayCoordinatorRecoversWorkerResultWhenGatewayCrashesBeforeExecuteCommit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 52
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingReferenceExecutor{result: ReferenceExecutionResult{
		Payload: json.RawMessage(`{"succeeded":true,"summary":"worker committed once"}`),
	}}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	baseTransport, err := NewInProcessTransport(worker, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	transport := &failAfterWorkerTransport{delegate: baseTransport, messageType: MessageExecute}
	controllerPath := filepath.Join(t.TempDir(), "controller.json")
	controller, err := OpenController(ControllerOptions{StatePath: controllerPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	input := RemoteRunInput{
		PlacementID: "placement-execute-crash", EnvironmentID: "environment-execute-crash",
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-execute-crash",
		Policy: DefaultExecutionPolicy(), Workspace: bundle, Request: json.RawMessage(`{"objective":"run once"}`),
	}
	if _, err := coordinator.Run(context.Background(), input); err == nil {
		t.Fatal("gateway execute crash was not injected")
	}
	if placement := controller.Snapshot().Placements[input.PlacementID]; placement.State != PlacementStateReady || placement.LastSequence != 3 {
		t.Fatalf("controller advanced past uncommitted execute: %+v", placement)
	}
	if executor.calls != 1 {
		t.Fatalf("worker execution calls before recovery=%d", executor.calls)
	}

	reopened, err := OpenController(ControllerOptions{StatePath: controllerPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: reopened, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: baseTransport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := recovered.RecoverPrepared(context.Background(), input)
	if err != nil || !result.Succeeded {
		t.Fatalf("recover prepared result=%+v err=%v", result, err)
	}
	if executor.calls != 1 {
		t.Fatalf("worker execution repeated after gateway crash: calls=%d", executor.calls)
	}
	if placement := reopened.Snapshot().Placements[input.PlacementID]; placement.State != PlacementStateDestroyed {
		t.Fatalf("recovered placement=%+v", placement)
	}
}

type failAfterWorkerTransport struct {
	delegate    WorkerTransport
	messageType MessageType
	failed      bool
}

func (transport *failAfterWorkerTransport) Exchange(ctx context.Context, request WireRequest) (WireResponse, error) {
	response, err := transport.delegate.Exchange(ctx, request)
	if err != nil {
		return response, err
	}
	if request.Envelope.Type == transport.messageType && !transport.failed {
		transport.failed = true
		return WireResponse{}, errors.New("simulated gateway crash after worker commit")
	}
	return response, nil
}

type recordThenFailRemoteResult struct {
	store *FileRemoteRunStore
	err   error
}

func (recorder *recordThenFailRemoteResult) RecordResult(ctx context.Context, input RemoteRunInput, result RemoteRunResult) error {
	if err := recorder.store.RecordResult(ctx, input, result); err != nil {
		return err
	}
	return recorder.err
}

func TestGatewayCoordinatorRunsAndAuditsRemotePlacementEndToEnd(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 17, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 11
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new task token issuer: %v", err)
	}
	executor := &recordingReferenceExecutor{result: ReferenceExecutionResult{
		Payload:    json.RawMessage(`{"succeeded":true,"summary":"done"}`),
		Artifacts:  []WireArtifact{{Name: "reports/result.txt", MediaType: "text/plain", Data: []byte("done\n"), Digest: digestBytes([]byte("done\n"))}},
		Checkpoint: &CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:checkpoint"},
	}}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new reference worker: %v", err)
	}
	transport, err := NewInProcessTransport(worker, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	controllerPath := filepath.Join(t.TempDir(), "controller.json")
	controller, err := OpenController(ControllerOptions{StatePath: controllerPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("open controller: %v", err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("new quarantine: %v", err)
	}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new gateway coordinator: %v", err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task\n"), 0o644); err != nil {
		t.Fatalf("write task source: %v", err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	result, err := coordinator.Run(context.Background(), RemoteRunInput{
		PlacementID: "placement-a", EnvironmentID: "env-a",
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		Policy: DefaultExecutionPolicy(), Workspace: bundle, Request: json.RawMessage(`{"objective":"complete task"}`),
		RedactValues: []string{"runtime-secret"},
	})
	if err != nil {
		t.Fatalf("run remote placement: %v", err)
	}
	if !result.Succeeded || len(result.Artifacts) != 1 || len(result.RejectedArtifacts) != 0 || result.Checkpoint == nil {
		t.Fatalf("remote result=%+v", result)
	}
	placement := controller.Snapshot().Placements["placement-a"]
	if placement.State != PlacementStateDestroyed || placement.LastSequence != 8 {
		t.Fatalf("final placement=%+v", placement)
	}
	if executor.calls != 1 || executor.request.Binding.AttemptID != "attempt-a" {
		t.Fatalf("executor calls=%d request=%+v", executor.calls, executor.request)
	}
	persisted, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read controller state: %v", err)
	}
	if bytes.Contains(persisted, []byte("complete task")) || bytes.Contains(persisted, []byte("tars-task-v1")) {
		t.Fatalf("controller persisted task content or token: %s", persisted)
	}
}

func TestGatewayCoordinatorFailsClosedWhenArtifactQuarantineRejectsSecret(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 17, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 12
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	secretArtifact := []byte("TOKEN=runtime-secret\n")
	executor := &recordingReferenceExecutor{result: ReferenceExecutionResult{
		Payload:   json.RawMessage(`{"succeeded":true}`),
		Artifacts: []WireArtifact{{Name: "result.txt", MediaType: "text/plain", Data: secretArtifact, Digest: digestBytes(secretArtifact)}},
	}}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewInProcessTransport(worker, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "controller.json"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-a", TransportName: "in-process", Endpoint: "local://worker-a",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Run(context.Background(), RemoteRunInput{
		PlacementID: "placement-a", EnvironmentID: "env-a", WorkspaceID: "workspace-a", WorkID: "work-a",
		StepID: "step-a", AttemptID: "attempt-a", Policy: DefaultExecutionPolicy(), Workspace: bundle,
	})
	if err != nil {
		t.Fatalf("remote run with rejected artifact should finish audibly: %v", err)
	}
	if result.Succeeded || len(result.Artifacts) != 0 || len(result.RejectedArtifacts) != 1 {
		t.Fatalf("quarantine did not fail closed: %+v", result)
	}
	if placement := controller.Snapshot().Placements["placement-a"]; placement.State != PlacementStateDestroyed {
		t.Fatalf("failed placement was not destroyed: %+v", placement)
	}
}

func TestGatewayCoordinatorResumesLostPlacementOnReplacementWorker(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 41
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "controller.json"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	registerReadyWorker(t, controller, "worker-a", 1000)
	if _, err := controller.CreatePlacement(context.Background(), CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		WorkerID: "worker-a", Policy: DefaultExecutionPolicy(),
		Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
	}); err != nil {
		t.Fatal(err)
	}
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 1, MessageProvision, ProvisionPayload{EnvironmentID: "env-a"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: "sha256:source"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 3, MessageLease, LeasePayload{LeaseTTLMS: 1000})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 4, MessageExecute, ExecutePayload{TaskToken: "old-ephemeral"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 5, MessageCheckpoint, CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:checkpoint"})
	now = now.Add(2 * time.Second)
	if lost, err := controller.ReconcileExpired(context.Background(), "worker-a heartbeat expired"); err != nil || len(lost) != 1 {
		t.Fatalf("mark worker-a lost: placements=%+v error=%v", lost, err)
	}

	executor := &recordingReferenceExecutor{result: ReferenceExecutionResult{
		Payload:   json.RawMessage(`{"succeeded":true,"summary":"resumed"}`),
		Artifacts: []WireArtifact{{Name: "resumed.txt", MediaType: "text/plain", Data: []byte("resumed\n"), Digest: digestBytes([]byte("resumed\n"))}},
	}}
	replacement, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-b", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewInProcessTransport(replacement, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-b", TransportName: "in-process", Endpoint: "local://worker-b",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: 2 * time.Minute, TokenTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("source snapshot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Resume(context.Background(), RemoteRecoveryInput{
		PlacementID: "placement-a", EnvironmentID: "env-b", Workspace: bundle,
		Request: json.RawMessage(`{"objective":"continue"}`), Reason: "replace expired worker",
	})
	if err != nil {
		t.Fatalf("resume placement on worker-b: %v", err)
	}
	if !result.Succeeded || len(result.Artifacts) != 1 || executor.calls != 1 || !executor.request.Resume || executor.request.Checkpoint == nil || executor.request.Checkpoint.ID != "checkpoint-a" {
		t.Fatalf("recovery result=%+v executor request=%+v calls=%d", result, executor.request, executor.calls)
	}
	snapshot := controller.Snapshot()
	placement := snapshot.Placements["placement-a"]
	if placement.State != PlacementStateDestroyed || placement.WorkerID != "worker-b" || placement.RecoveryCount != 1 || placement.LastSequence != 12 {
		t.Fatalf("recovered placement=%+v", placement)
	}
	if snapshot.Workers["worker-a"].State != WorkerStateLost || snapshot.Workers["worker-b"].State != WorkerStateReady {
		t.Fatalf("recovery workers=%+v", snapshot.Workers)
	}
}
