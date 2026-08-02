package workerprotocol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/workstore"
)

type WorkLedgerSink struct {
	store   *workstore.Store
	actorID string
}

func NewWorkLedgerSink(store *workstore.Store, actorID string) (*WorkLedgerSink, error) {
	if store == nil || strings.TrimSpace(actorID) == "" {
		return nil, fmt.Errorf("workerprotocol: work ledger and actor are required")
	}
	return &WorkLedgerSink{store: store, actorID: strings.TrimSpace(actorID)}, nil
}

func (sink *WorkLedgerSink) Record(ctx context.Context, event ControlEvent) error {
	if sink == nil || sink.store == nil {
		return fmt.Errorf("workerprotocol: work ledger sink is not configured")
	}
	// Worker registration and idle heartbeats are retained in the controller's
	// atomic audit log. Work-scoped placement events additionally enter the Work
	// Ledger so scheduling and placement share one timeline.
	if strings.TrimSpace(event.WorkID) == "" {
		return nil
	}
	eventType, ok := workLedgerEventType(event.Type)
	if !ok {
		return fmt.Errorf("workerprotocol: unsupported control event %q", event.Type)
	}
	payload, err := json.Marshal(workLedgerEventPayload(event))
	if err != nil {
		return fmt.Errorf("workerprotocol: encode work ledger event: %w", err)
	}
	_, err = sink.store.RecordExecutionEvent(ctx, workstore.RecordExecutionEventInput{
		WorkspaceID: event.WorkspaceID, WorkID: event.WorkID, StepID: event.StepID,
		AttemptID: event.AttemptID, Type: eventType, ActorID: sink.actorID,
		CausationID: event.AttemptID, IdempotencyKey: "worker-protocol:" + event.IdempotencyKey,
		PayloadJSON: payload,
	})
	return err
}

func workLedgerEventType(raw string) (workstore.EventType, bool) {
	switch raw {
	case "placement.created":
		return workstore.EventTypeWorkerPlacementCreated, true
	case string(MessageProvision):
		return workstore.EventTypeWorkerEnvironmentProvisioned, true
	case string(MessageSync):
		return workstore.EventTypeWorkerWorkspaceSynced, true
	case string(MessageLease):
		return workstore.EventTypeWorkerLeaseGranted, true
	case string(MessageHeartbeat):
		return workstore.EventTypeWorkerHeartbeatObserved, true
	case string(MessageExecute):
		return workstore.EventTypeWorkerExecutionStarted, true
	case string(MessageStream):
		return workstore.EventTypeWorkerStreamObserved, true
	case string(MessageCheckpoint):
		return workstore.EventTypeWorkerCheckpointRecorded, true
	case string(MessageCollect):
		return workstore.EventTypeWorkerArtifactsCollected, true
	case string(MessageDestroy):
		return workstore.EventTypeWorkerPlacementDestroyed, true
	case string(MessageLost):
		return workstore.EventTypeWorkerLost, true
	case string(MessageReclaim):
		return workstore.EventTypeWorkerReclaimed, true
	case string(MessageRehydrate):
		return workstore.EventTypeWorkerRehydrated, true
	default:
		return "", false
	}
}

func workLedgerEventPayload(event ControlEvent) map[string]any {
	payload := map[string]any{
		"protocol_version": ProtocolVersionV1,
		"message_id":       event.MessageID,
		"event_type":       event.Type,
		"worker_id":        event.WorkerID,
		"placement_id":     event.PlacementID,
		"sequence":         event.Sequence,
		"from_state":       event.FromState,
		"to_state":         event.ToState,
	}
	if len(event.Payload) == 0 {
		return payload
	}
	// ControlEvent payloads are already sanitized by Controller. Hash again and
	// only project a narrow allowlist so a custom sink caller cannot persist a
	// bearer token or arbitrary task content.
	digest := sha256.Sum256(event.Payload)
	payload["payload_digest"] = "sha256:" + hex.EncodeToString(digest[:])
	var raw map[string]any
	if json.Unmarshal(event.Payload, &raw) != nil {
		return payload
	}
	for _, key := range safeLedgerPayloadKeys(event.Type) {
		if value, ok := raw[key]; ok {
			payload[key] = value
		}
	}
	return payload
}

func safeLedgerPayloadKeys(eventType string) []string {
	switch eventType {
	case string(MessageProvision):
		return []string{"environment_id", "manifest_digest", "policy"}
	case string(MessageSync):
		return []string{"mode", "digest", "file_count", "total_bytes"}
	case string(MessageLease), string(MessageHeartbeat):
		return []string{"lease_ttl_ms", "usage"}
	case string(MessageExecute):
		return []string{"resume", "checkpoint_id", "checkpoint_digest", "request_digest"}
	case string(MessageStream):
		return []string{"kind", "text_bytes", "payload_digest"}
	case string(MessageCheckpoint):
		return []string{"checkpoint_id", "digest", "uri_digest"}
	case string(MessageCollect):
		return []string{"complete", "succeeded", "snapshot_digest", "artifact_count"}
	case string(MessageRehydrate):
		return []string{"replacement_worker_id", "environment_id", "snapshot_digest", "checkpoint_id", "checkpoint_digest"}
	default:
		return nil
	}
}

var _ EventSink = (*WorkLedgerSink)(nil)
