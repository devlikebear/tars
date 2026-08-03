package workerprotocol

import (
	"context"
	"testing"
	"time"
)

func TestControllerReclaimsExpiredWorkerAndRehydratesCheckpointOnReplacement(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 19, 0, 0, 0, time.UTC)
	controller, err := OpenController(ControllerOptions{StatePath: t.TempDir() + "/state.json", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	registerReadyWorker(t, controller, "worker-a", 1000)
	registerReadyWorker(t, controller, "worker-b", 0)
	if _, err := controller.CreatePlacement(context.Background(), CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		WorkerID: "worker-a", Policy: DefaultExecutionPolicy(),
		Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
	}); err != nil {
		t.Fatal(err)
	}
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 1, MessageProvision, ProvisionPayload{EnvironmentID: "env-a"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: "sha256:workspace"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 3, MessageLease, LeasePayload{LeaseTTLMS: 1000})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 4, MessageExecute, ExecutePayload{TaskToken: "ephemeral"})
	applyRecoveryEnvelope(t, controller, "worker-a", "placement-a", 5, MessageCheckpoint, CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:checkpoint"})

	now = now.Add(2 * time.Second)
	lost, err := controller.ReconcileExpired(context.Background(), "heartbeat lease expired")
	if err != nil {
		t.Fatalf("reconcile expired worker: %v", err)
	}
	if len(lost) != 1 || lost[0].ID != "placement-a" || lost[0].State != PlacementStateLost {
		t.Fatalf("lost placements=%+v", lost)
	}
	if snapshot := controller.Snapshot(); snapshot.Workers["worker-a"].State != WorkerStateLost || snapshot.Placements["placement-a"].LastSequence != 6 {
		t.Fatalf("lost snapshot=%+v", snapshot)
	}
	if repeated, err := controller.ReconcileExpired(context.Background(), "duplicate monitor tick"); err != nil || len(repeated) != 0 {
		t.Fatalf("duplicate reconcile placements=%+v error=%v", repeated, err)
	}

	reclaiming, err := controller.BeginReclaim(context.Background(), "placement-a", "replace lost worker")
	if err != nil || reclaiming.State != PlacementStateReclaiming || reclaiming.LastSequence != 7 {
		t.Fatalf("begin reclaim placement=%+v error=%v", reclaiming, err)
	}
	rehydrated, err := controller.RehydratePlacement(context.Background(), RehydratePlacementInput{
		PlacementID: "placement-a", ReplacementWorkerID: "worker-b", EnvironmentID: "env-b",
		SnapshotDigest: "sha256:snapshot", CheckpointID: "checkpoint-a", CheckpointDigest: "sha256:checkpoint",
		LeaseTTLMS: 60_000,
	})
	if err != nil {
		t.Fatalf("rehydrate placement: %v", err)
	}
	if rehydrated.State != PlacementStateRehydrating || rehydrated.WorkerID != "worker-b" || rehydrated.RecoveryCount != 1 || rehydrated.LastSequence != 8 {
		t.Fatalf("rehydrated placement=%+v", rehydrated)
	}
	applyRecoveryEnvelope(t, controller, "worker-b", "placement-a", 9, MessageExecute, ExecutePayload{
		TaskToken: "fresh-ephemeral", Resume: true, CheckpointID: "checkpoint-a", CheckpointHash: "sha256:checkpoint",
	})
	final := controller.Snapshot()
	if final.Placements["placement-a"].State != PlacementStateExecuting || final.Workers["worker-b"].State != WorkerStateExecuting {
		t.Fatalf("resumed snapshot=%+v", final)
	}
}

func registerReadyWorker(t *testing.T, controller *Controller, workerID string, heartbeatTTLMS int64) {
	t.Helper()
	if _, err := controller.Apply(context.Background(), testEnvelope(workerID, "", 1, MessageRegister, RegisterPayload{
		Transport: "in-process", Endpoint: "local://" + workerID,
		Capabilities: WorkerCapabilities{Resume: true, Checkpoints: true, EgressPolicy: true, ResourceLimits: true},
	})); err != nil {
		t.Fatalf("register %s: %v", workerID, err)
	}
	if _, err := controller.Apply(context.Background(), testEnvelope(workerID, "", 2, MessageHeartbeat, HeartbeatPayload{LeaseTTLMS: heartbeatTTLMS})); err != nil {
		t.Fatalf("ready %s: %v", workerID, err)
	}
}

func applyRecoveryEnvelope(t *testing.T, controller *Controller, workerID, placementID string, sequence int64, messageType MessageType, payload any) {
	t.Helper()
	if _, err := controller.Apply(context.Background(), testEnvelope(workerID, placementID, sequence, messageType, payload)); err != nil {
		t.Fatalf("apply %s sequence %d: %v", messageType, sequence, err)
	}
}
