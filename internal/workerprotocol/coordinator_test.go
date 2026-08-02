package workerprotocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
