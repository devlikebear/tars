package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControllerPersistsLifecycleAndPublishesEachMessageOnce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "worker-control.json")
	sink := &recordingSink{}
	controller, err := OpenController(ControllerOptions{
		StatePath: path,
		Now:       func() time.Time { return now },
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("open controller: %v", err)
	}

	apply := func(envelope Envelope) ApplyResult {
		t.Helper()
		result, applyErr := controller.Apply(context.Background(), envelope)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", envelope.Type, applyErr)
		}
		return result
	}
	workerEnvelope := func(sequence int64, messageType MessageType, payload any) Envelope {
		return testEnvelope("worker-a", "", sequence, messageType, payload)
	}
	apply(workerEnvelope(1, MessageRegister, RegisterPayload{
		Transport: "in-process", Endpoint: "local://worker-a",
		Capabilities: WorkerCapabilities{Resume: true, EgressPolicy: true, ResourceLimits: true},
	}))
	apply(workerEnvelope(2, MessageHeartbeat, HeartbeatPayload{LeaseTTLMS: 30_000}))
	if worker := controller.Snapshot().Workers["worker-a"]; worker.State != WorkerStateReady {
		t.Fatalf("worker after heartbeat = %+v", worker)
	}

	placement, err := controller.CreatePlacement(context.Background(), CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a",
		StepID: "step-a", AttemptID: "attempt-a", WorkerID: "worker-a",
		Policy: DefaultExecutionPolicy(), Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
	})
	if err != nil {
		t.Fatalf("create placement: %v", err)
	}
	if placement.State != PlacementStatePending {
		t.Fatalf("created placement state=%s want pending", placement.State)
	}
	placementEnvelope := func(sequence int64, messageType MessageType, payload any) Envelope {
		return testEnvelope("worker-a", "placement-a", sequence, messageType, payload)
	}
	apply(placementEnvelope(1, MessageProvision, ProvisionPayload{EnvironmentID: "env-a"}))
	apply(placementEnvelope(2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: "sha256:workspace"}))
	apply(placementEnvelope(3, MessageLease, LeasePayload{LeaseTTLMS: 30_000}))
	apply(placementEnvelope(4, MessageExecute, ExecutePayload{TaskToken: "task-token", Resume: false}))
	apply(placementEnvelope(5, MessageStream, StreamPayload{Kind: "progress", Text: "halfway"}))
	apply(placementEnvelope(6, MessageCheckpoint, CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:checkpoint"}))
	apply(placementEnvelope(7, MessageExecute, ExecutePayload{TaskToken: "task-token", Resume: true}))
	apply(placementEnvelope(8, MessageCollect, CollectPayload{Complete: false}))
	apply(placementEnvelope(9, MessageCollect, CollectPayload{Complete: true, Succeeded: true, SnapshotDigest: "sha256:result"}))
	destroy := placementEnvelope(10, MessageDestroy, DestroyPayload{Reason: "completed"})
	result := apply(destroy)
	if result.Duplicate {
		t.Fatal("first destroy was marked duplicate")
	}
	duplicate := apply(destroy)
	if !duplicate.Duplicate {
		t.Fatal("duplicate destroy was applied twice")
	}

	snapshot := controller.Snapshot()
	if snapshot.Placements["placement-a"].State != PlacementStateDestroyed {
		t.Fatalf("placement = %+v", snapshot.Placements["placement-a"])
	}
	if snapshot.Workers["worker-a"].State != WorkerStateReady {
		t.Fatalf("worker after destroy = %+v", snapshot.Workers["worker-a"])
	}
	if len(sink.events) != 13 { // register + heartbeat + placement creation + 10 placement messages
		t.Fatalf("published event count=%d want 13", len(sink.events))
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted controller state: %v", err)
	}
	if bytes.Contains(persisted, []byte("task-token")) {
		t.Fatal("task-scoped bearer token was persisted in controller state")
	}

	reopened, err := OpenController(ControllerOptions{StatePath: path, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("reopen controller: %v", err)
	}
	restored := reopened.Snapshot()
	if restored.Placements["placement-a"].LastSequence != 10 || restored.Placements["placement-a"].State != PlacementStateDestroyed {
		t.Fatalf("restored placement = %+v", restored.Placements["placement-a"])
	}
}

func TestControllerRejectsReorderedMessagesWithoutMutation(t *testing.T) {
	t.Parallel()

	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "state.json")})
	if err != nil {
		t.Fatalf("open controller: %v", err)
	}
	if _, err := controller.Apply(context.Background(), testEnvelope("worker-a", "", 1, MessageRegister, RegisterPayload{
		Transport: "in-process", Endpoint: "local://worker-a",
		Capabilities: WorkerCapabilities{EgressPolicy: true, ResourceLimits: true},
	})); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	if _, err := controller.Apply(context.Background(), testEnvelope("worker-a", "", 2, MessageHeartbeat, HeartbeatPayload{})); err != nil {
		t.Fatalf("ready worker: %v", err)
	}
	if _, err := controller.CreatePlacement(context.Background(), CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		WorkerID: "worker-a", Policy: DefaultExecutionPolicy(),
		Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
	}); err != nil {
		t.Fatalf("create placement: %v", err)
	}

	outOfOrder := testEnvelope("worker-a", "placement-a", 2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: "sha256:workspace"})
	if _, err := controller.Apply(context.Background(), outOfOrder); !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("reordered event error=%v want ErrOutOfOrder", err)
	}
	unchanged := controller.Snapshot().Placements["placement-a"]
	if unchanged.State != PlacementStatePending || unchanged.LastSequence != 0 {
		t.Fatalf("reordered event mutated placement = %+v", unchanged)
	}
	if _, err := controller.Apply(context.Background(), testEnvelope("worker-a", "placement-a", 1, MessageProvision, ProvisionPayload{EnvironmentID: "env-a"})); err != nil {
		t.Fatalf("apply missing first message: %v", err)
	}
	if _, err := controller.Apply(context.Background(), outOfOrder); err != nil {
		t.Fatalf("apply reordered message after gap filled: %v", err)
	}
	if placement := controller.Snapshot().Placements["placement-a"]; placement.State != PlacementStateSyncing || placement.LastSequence != 2 {
		t.Fatalf("placement after ordered replay = %+v", placement)
	}
}

func TestControllerRollsBackInMemoryStateWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	t.Run("apply", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), "state.json")
		controller, err := OpenController(ControllerOptions{StatePath: statePath})
		if err != nil {
			t.Fatalf("open controller: %v", err)
		}
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("create persistence blocker: %v", err)
		}
		controller.statePath = filepath.Join(blocked, "state.json")
		envelope := testEnvelope("worker-a", "", 1, MessageRegister, RegisterPayload{
			Transport: "in-process", Endpoint: "local://worker-a",
			Capabilities: WorkerCapabilities{EgressPolicy: true, ResourceLimits: true},
		})
		if _, err := controller.Apply(context.Background(), envelope); err == nil {
			t.Fatal("apply succeeded despite persistence failure")
		}
		if snapshot := controller.Snapshot(); len(snapshot.Workers) != 0 || len(snapshot.Events) != 0 {
			t.Fatalf("failed apply mutated controller state: %+v", snapshot)
		}
		controller.statePath = statePath
		if _, err := controller.Apply(context.Background(), envelope); err != nil {
			t.Fatalf("retry after persistence recovery: %v", err)
		}
	})

	t.Run("create placement", func(t *testing.T) {
		statePath := filepath.Join(t.TempDir(), "state.json")
		controller, err := OpenController(ControllerOptions{StatePath: statePath})
		if err != nil {
			t.Fatalf("open controller: %v", err)
		}
		if _, err := controller.Apply(context.Background(), testEnvelope("worker-a", "", 1, MessageRegister, RegisterPayload{
			Transport: "in-process", Endpoint: "local://worker-a",
			Capabilities: WorkerCapabilities{EgressPolicy: true, ResourceLimits: true},
		})); err != nil {
			t.Fatalf("register worker: %v", err)
		}
		if _, err := controller.Apply(context.Background(), testEnvelope("worker-a", "", 2, MessageHeartbeat, HeartbeatPayload{})); err != nil {
			t.Fatalf("ready worker: %v", err)
		}
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatalf("create persistence blocker: %v", err)
		}
		input := CreatePlacementInput{
			ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
			WorkerID: "worker-a", Policy: DefaultExecutionPolicy(),
			Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
		}
		controller.statePath = filepath.Join(blocked, "state.json")
		if _, err := controller.CreatePlacement(context.Background(), input); err == nil {
			t.Fatal("placement creation succeeded despite persistence failure")
		}
		if snapshot := controller.Snapshot(); len(snapshot.Placements) != 0 {
			t.Fatalf("failed placement creation mutated controller state: %+v", snapshot.Placements)
		}
		controller.statePath = statePath
		if _, err := controller.CreatePlacement(context.Background(), input); err != nil {
			t.Fatalf("retry placement after persistence recovery: %v", err)
		}
	})
}

func testEnvelope(workerID, placementID string, sequence int64, messageType MessageType, payload any) Envelope {
	raw, _ := json.Marshal(payload)
	scope := workerID
	if placementID != "" {
		scope = placementID
	}
	return Envelope{
		ProtocolVersion: ProtocolVersionV1,
		MessageID:       scope + ":msg:" + messageType.String() + ":" + time.Unix(sequence, 0).UTC().Format(time.RFC3339Nano),
		IdempotencyKey:  scope + ":idem:" + messageType.String() + ":" + time.Unix(sequence, 0).UTC().Format(time.RFC3339Nano),
		Type:            messageType,
		WorkerID:        workerID,
		PlacementID:     placementID,
		Sequence:        sequence,
		SentAt:          time.Unix(sequence, 0).UTC(),
		Payload:         raw,
	}
}

type recordingSink struct {
	events []ControlEvent
}

func (sink *recordingSink) Record(_ context.Context, event ControlEvent) error {
	sink.events = append(sink.events, event)
	return nil
}
