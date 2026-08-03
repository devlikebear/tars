package executionplane

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

type WorkLedgerSink struct {
	store   *workstore.Store
	actorID string
}

func NewWorkLedgerSink(store *workstore.Store, actorID string) (*WorkLedgerSink, error) {
	if store == nil || strings.TrimSpace(actorID) == "" {
		return nil, fmt.Errorf("executionplane: work ledger and actor are required")
	}
	return &WorkLedgerSink{store: store, actorID: strings.TrimSpace(actorID)}, nil
}

func (sink *WorkLedgerSink) Record(ctx context.Context, event LifecycleEvent) error {
	eventType, ok := ledgerEventType(event.Phase)
	if !ok {
		return fmt.Errorf("executionplane: unsupported lifecycle event %q", event.Phase)
	}
	execution := event.Execution
	if strings.TrimSpace(execution.Work.ID) == "" || strings.TrimSpace(execution.Claim.Step.ID) == "" || strings.TrimSpace(execution.Claim.Attempt.ID) == "" {
		return fmt.Errorf("executionplane: lifecycle event is missing durable execution identity")
	}
	payload, err := json.Marshal(map[string]any{
		"phase": event.Phase, "provider": strings.TrimSpace(event.Provider),
		"worker": strings.TrimSpace(event.Worker), "environment_id": strings.TrimSpace(event.EnvironmentID),
		"credential_id": strings.TrimSpace(event.CredentialID), "checkpoint_id": strings.TrimSpace(event.CheckpointID),
		"artifact_count": event.ArtifactCount, "snapshot": event.Snapshot,
	})
	if err != nil {
		return fmt.Errorf("executionplane: encode lifecycle event: %w", err)
	}
	idempotencyKey := lifecycleEventKey(execution.Claim.Attempt.ID, event)
	_, err = sink.store.RecordExecutionEvent(ctx, workstore.RecordExecutionEventInput{
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID,
		StepID: execution.Claim.Step.ID, AttemptID: execution.Claim.Attempt.ID,
		Type: eventType, ActorID: sink.actorID, CausationID: execution.Claim.Attempt.ID,
		IdempotencyKey: idempotencyKey, PayloadJSON: payload,
	})
	return err
}

func (sink *WorkLedgerSink) StoreArtifact(ctx context.Context, execution workscheduler.Execution, artifact CollectedArtifact) error {
	keyMaterial := strings.Join([]string{artifact.Kind, artifact.Name, artifact.URI, artifact.Digest}, "\x00")
	digest := sha256.Sum256([]byte(keyMaterial))
	_, err := sink.store.CreateArtifact(ctx, workstore.CreateArtifactInput{
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID,
		StepID: execution.Claim.Step.ID, AttemptID: execution.Claim.Attempt.ID,
		IdempotencyKey: fmt.Sprintf("%s:execution-artifact:%x", execution.Claim.Attempt.ID, digest[:]),
		CausationID:    execution.Claim.Attempt.ID, Kind: strings.TrimSpace(artifact.Kind),
		Name: strings.TrimSpace(artifact.Name), URI: strings.TrimSpace(artifact.URI),
		Digest: strings.TrimSpace(artifact.Digest), MediaType: strings.TrimSpace(artifact.MediaType),
		SizeBytes: artifact.SizeBytes, ActorID: sink.actorID,
	})
	return err
}

func lifecycleEventKey(attemptID string, event LifecycleEvent) string {
	discriminator := strings.Join([]string{
		string(event.Phase), event.EnvironmentID, event.CredentialID,
		event.CheckpointID, event.Snapshot.Digest, fmt.Sprintf("%d", event.ArtifactCount),
	}, "\x00")
	digest := sha256.Sum256([]byte(discriminator))
	return fmt.Sprintf("%s:execution-event:%x", strings.TrimSpace(attemptID), digest[:])
}

func ledgerEventType(phase EventPhase) (workstore.EventType, bool) {
	switch phase {
	case EventEnvironmentProvisioned:
		return workstore.EventTypeExecutionEnvironmentProvisioned, true
	case EventCredentialsIssued:
		return workstore.EventTypeExecutionCredentialsIssued, true
	case EventWorkerStarted:
		return workstore.EventTypeExecutionWorkerStarted, true
	case EventCheckpointRecorded:
		return workstore.EventTypeExecutionCheckpointRecorded, true
	case EventEnvironmentSynced:
		return workstore.EventTypeExecutionEnvironmentSynced, true
	case EventArtifactsCollected:
		return workstore.EventTypeExecutionArtifactsCollected, true
	case EventCredentialsRevoked:
		return workstore.EventTypeExecutionCredentialsRevoked, true
	case EventEnvironmentDestroyed:
		return workstore.EventTypeExecutionEnvironmentDestroyed, true
	case EventRecoveryStarted:
		return workstore.EventTypeExecutionRecoveryStarted, true
	case EventWorkerCancelled:
		return workstore.EventTypeExecutionWorkerCancelled, true
	default:
		return "", false
	}
}

var _ EventSink = (*WorkLedgerSink)(nil)
var _ ArtifactSink = (*WorkLedgerSink)(nil)
