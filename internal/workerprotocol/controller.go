package workerprotocol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const controllerSchemaVersion = 1

type EventSink interface {
	Record(context.Context, ControlEvent) error
}

type ControlEvent struct {
	ID             string          `json:"id"`
	MessageID      string          `json:"message_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	Type           string          `json:"type"`
	Entity         string          `json:"entity"`
	WorkerID       string          `json:"worker_id,omitempty"`
	PlacementID    string          `json:"placement_id,omitempty"`
	WorkspaceID    string          `json:"workspace_id,omitempty"`
	WorkID         string          `json:"work_id,omitempty"`
	StepID         string          `json:"step_id,omitempty"`
	AttemptID      string          `json:"attempt_id,omitempty"`
	Sequence       int64           `json:"sequence,omitempty"`
	FromState      string          `json:"from_state,omitempty"`
	ToState        string          `json:"to_state,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Published      bool            `json:"published"`
	OccurredAt     time.Time       `json:"occurred_at"`
}

type ControllerSnapshot struct {
	SchemaVersion int                  `json:"schema_version"`
	Workers       map[string]Worker    `json:"workers"`
	Placements    map[string]Placement `json:"placements"`
	Events        []ControlEvent       `json:"events"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type controllerState struct {
	SchemaVersion          int                  `json:"schema_version"`
	Workers                map[string]Worker    `json:"workers"`
	Placements             map[string]Placement `json:"placements"`
	Events                 []ControlEvent       `json:"events"`
	AppliedMessages        map[string]string    `json:"applied_messages"`
	AppliedIdempotencyKeys map[string]string    `json:"applied_idempotency_keys"`
	UpdatedAt              time.Time            `json:"updated_at"`
}

type ControllerOptions struct {
	StatePath string
	EventSink EventSink
	Now       func() time.Time
}

type Controller struct {
	mu        sync.Mutex
	statePath string
	eventSink EventSink
	now       func() time.Time
	state     controllerState
}

type CreatePlacementInput struct {
	ID          string
	WorkspaceID string
	WorkID      string
	StepID      string
	AttemptID   string
	WorkerID    string
	Policy      ExecutionPolicy
	Sync        SyncSpec
}

type ApplyResult struct {
	Duplicate bool         `json:"duplicate"`
	Worker    *Worker      `json:"worker,omitempty"`
	Placement *Placement   `json:"placement,omitempty"`
	Event     ControlEvent `json:"event"`
}

func OpenController(opts ControllerOptions) (*Controller, error) {
	if strings.TrimSpace(opts.StatePath) == "" {
		return nil, fmt.Errorf("workerprotocol: controller state path is required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	controller := &Controller{
		statePath: strings.TrimSpace(opts.StatePath), eventSink: opts.EventSink, now: opts.Now,
		state: newControllerState(),
	}
	raw, err := os.ReadFile(controller.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return controller, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: read controller state: %w", err)
	}
	if err := json.Unmarshal(raw, &controller.state); err != nil {
		return nil, fmt.Errorf("workerprotocol: decode controller state: %w", err)
	}
	if controller.state.SchemaVersion != controllerSchemaVersion {
		return nil, fmt.Errorf("%w: controller schema %d", ErrVersionUnsupported, controller.state.SchemaVersion)
	}
	controller.initializeMapsLocked()
	return controller, nil
}

func newControllerState() controllerState {
	return controllerState{
		SchemaVersion: controllerSchemaVersion,
		Workers:       make(map[string]Worker), Placements: make(map[string]Placement),
		Events: make([]ControlEvent, 0), AppliedMessages: make(map[string]string),
		AppliedIdempotencyKeys: make(map[string]string),
	}
}

func (controller *Controller) CreatePlacement(ctx context.Context, input CreatePlacementInput) (Placement, error) {
	if controller == nil {
		return Placement{}, fmt.Errorf("workerprotocol: controller is nil")
	}
	if !validProtocolIdentifier(input.ID) || !validProtocolIdentifier(input.WorkspaceID) ||
		!validProtocolIdentifier(input.WorkID) || !validProtocolIdentifier(input.StepID) ||
		!validProtocolIdentifier(input.AttemptID) || !validProtocolIdentifier(input.WorkerID) {
		return Placement{}, fmt.Errorf("%w: invalid placement identity", ErrInvalidEnvelope)
	}
	if err := input.Policy.Validate(); err != nil {
		return Placement{}, err
	}
	if err := input.Sync.Validate(); err != nil {
		return Placement{}, err
	}

	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.initializeMapsLocked()
	if existing, ok := controller.state.Placements[input.ID]; ok {
		if existing.WorkspaceID == input.WorkspaceID && existing.WorkID == input.WorkID && existing.StepID == input.StepID && existing.AttemptID == input.AttemptID && existing.WorkerID == input.WorkerID {
			return existing, nil
		}
		return Placement{}, fmt.Errorf("%w: placement %s already exists", ErrConflict, input.ID)
	}
	for _, existing := range controller.state.Placements {
		if existing.WorkspaceID == input.WorkspaceID && existing.AttemptID == input.AttemptID && existing.State != PlacementStateDestroyed {
			return Placement{}, fmt.Errorf("%w: attempt %s already has placement %s", ErrConflict, input.AttemptID, existing.ID)
		}
	}
	worker, ok := controller.state.Workers[input.WorkerID]
	if !ok {
		return Placement{}, ErrNotFound
	}
	if worker.State != WorkerStateReady {
		return Placement{}, fmt.Errorf("%w: worker %s is %s", ErrInvalidTransition, worker.ID, worker.State)
	}
	if !worker.Capabilities.EgressPolicy || !worker.Capabilities.ResourceLimits {
		return Placement{}, fmt.Errorf("%w: worker %s cannot enforce placement policy", ErrInvalidPolicy, worker.ID)
	}
	previousState := cloneControllerState(controller.state)
	now := controller.now().UTC()
	placement := Placement{
		ID: input.ID, WorkspaceID: input.WorkspaceID, WorkID: input.WorkID,
		StepID: input.StepID, AttemptID: input.AttemptID, WorkerID: input.WorkerID,
		State: PlacementStatePending, Policy: input.Policy, Sync: input.Sync,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	controller.state.Placements[placement.ID] = placement
	event := ControlEvent{
		ID: "placement-created:" + placement.ID, IdempotencyKey: "placement-created:" + placement.ID,
		Type: "placement.created", Entity: "placement", WorkerID: placement.WorkerID,
		PlacementID: placement.ID, WorkspaceID: placement.WorkspaceID, WorkID: placement.WorkID,
		StepID: placement.StepID, AttemptID: placement.AttemptID,
		ToState: string(placement.State), OccurredAt: now,
	}
	eventIndex, err := controller.appendAndSaveLocked(event)
	if err != nil {
		controller.state = previousState
		return Placement{}, err
	}
	if err := controller.publishEventLocked(ctx, eventIndex); err != nil {
		return placement, err
	}
	return placement, nil
}

func (controller *Controller) Apply(ctx context.Context, envelope Envelope) (ApplyResult, error) {
	if controller == nil {
		return ApplyResult{}, fmt.Errorf("workerprotocol: controller is nil")
	}
	if err := envelope.Validate(); err != nil {
		return ApplyResult{}, err
	}
	fingerprint := envelopeFingerprint(envelope)

	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.initializeMapsLocked()
	if existing, ok := controller.state.AppliedMessages[envelope.MessageID]; ok {
		if existing != fingerprint {
			return ApplyResult{}, fmt.Errorf("%w: message id %s changed payload", ErrConflict, envelope.MessageID)
		}
		return controller.duplicateResultLocked(ctx, envelope), nil
	}
	if existing, ok := controller.state.AppliedIdempotencyKeys[envelope.IdempotencyKey]; ok {
		if existing != fingerprint {
			return ApplyResult{}, fmt.Errorf("%w: idempotency key %s changed payload", ErrConflict, envelope.IdempotencyKey)
		}
		return controller.duplicateResultLocked(ctx, envelope), nil
	}
	previousState := cloneControllerState(controller.state)

	var event ControlEvent
	var worker *Worker
	var placement *Placement
	var err error
	if envelope.PlacementID == "" {
		event, worker, err = controller.applyWorkerMessageLocked(envelope)
	} else {
		event, worker, placement, err = controller.applyPlacementMessageLocked(envelope)
	}
	if err != nil {
		return ApplyResult{}, err
	}
	controller.state.AppliedMessages[envelope.MessageID] = fingerprint
	controller.state.AppliedIdempotencyKeys[envelope.IdempotencyKey] = fingerprint
	eventIndex, err := controller.appendAndSaveLocked(event)
	if err != nil {
		controller.state = previousState
		return ApplyResult{}, err
	}
	if err := controller.publishEventLocked(ctx, eventIndex); err != nil {
		return ApplyResult{Worker: worker, Placement: placement, Event: event}, err
	}
	return ApplyResult{Worker: worker, Placement: placement, Event: controller.state.Events[eventIndex]}, nil
}

func (controller *Controller) Snapshot() ControllerSnapshot {
	if controller == nil {
		return ControllerSnapshot{Workers: map[string]Worker{}, Placements: map[string]Placement{}, Events: []ControlEvent{}}
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.initializeMapsLocked()
	raw, _ := json.Marshal(ControllerSnapshot{
		SchemaVersion: controller.state.SchemaVersion, Workers: controller.state.Workers,
		Placements: controller.state.Placements, Events: controller.state.Events, UpdatedAt: controller.state.UpdatedAt,
	})
	var snapshot ControllerSnapshot
	_ = json.Unmarshal(raw, &snapshot)
	return snapshot
}

func (controller *Controller) FlushPending(ctx context.Context) error {
	if controller == nil {
		return nil
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	for index := range controller.state.Events {
		if controller.state.Events[index].Published {
			continue
		}
		if err := controller.publishEventLocked(ctx, index); err != nil {
			return err
		}
	}
	return nil
}

func (controller *Controller) applyWorkerMessageLocked(envelope Envelope) (ControlEvent, *Worker, error) {
	now := controller.now().UTC()
	current, exists := controller.state.Workers[envelope.WorkerID]
	if exists && envelope.Sequence != current.LastSequence+1 {
		return ControlEvent{}, nil, fmt.Errorf("%w: worker %s expected sequence %d, got %d", ErrOutOfOrder, envelope.WorkerID, current.LastSequence+1, envelope.Sequence)
	}
	if !exists && envelope.Sequence != 1 {
		return ControlEvent{}, nil, fmt.Errorf("%w: new worker %s must start at sequence 1", ErrOutOfOrder, envelope.WorkerID)
	}
	fromState := current.State
	switch envelope.Type {
	case MessageRegister:
		var payload RegisterPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || strings.TrimSpace(payload.Transport) == "" || strings.TrimSpace(payload.Endpoint) == "" || !payload.Capabilities.EgressPolicy || !payload.Capabilities.ResourceLimits {
			return ControlEvent{}, nil, fmt.Errorf("%w: invalid worker registration", ErrInvalidEnvelope)
		}
		if exists && current.State != WorkerStateLost && current.State != WorkerStateDisconnected && current.State != WorkerStateDestroyed {
			return ControlEvent{}, nil, fmt.Errorf("%w: worker %s cannot register from %s", ErrInvalidTransition, current.ID, current.State)
		}
		if !exists {
			current = Worker{ID: envelope.WorkerID, CreatedAt: now}
		}
		current.ProtocolVersion = envelope.ProtocolVersion
		current.Transport = strings.TrimSpace(payload.Transport)
		current.Endpoint = strings.TrimSpace(payload.Endpoint)
		current.Capabilities = payload.Capabilities
		current.State = WorkerStateRegistered
		current.LeaseExpiresAt = nil
	case MessageHeartbeat:
		if !exists {
			return ControlEvent{}, nil, ErrNotFound
		}
		if current.State != WorkerStateRegistered && current.State != WorkerStateReady && current.State != WorkerStateLeased && current.State != WorkerStateExecuting {
			return ControlEvent{}, nil, fmt.Errorf("%w: worker %s heartbeat from %s", ErrInvalidTransition, current.ID, current.State)
		}
		var payload HeartbeatPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return ControlEvent{}, nil, err
		}
		if current.State == WorkerStateRegistered {
			current.State = WorkerStateReady
		}
		if payload.LeaseTTLMS > 0 {
			expires := now.Add(time.Duration(payload.LeaseTTLMS) * time.Millisecond)
			current.LeaseExpiresAt = &expires
		}
	default:
		return ControlEvent{}, nil, fmt.Errorf("%w: %s requires a placement", ErrInvalidEnvelope, envelope.Type)
	}
	current.LastSequence = envelope.Sequence
	current.LastSeenAt = now
	current.UpdatedAt = now
	current.Version++
	controller.state.Workers[current.ID] = current
	copy := current
	return controller.eventForEnvelope(envelope, "worker", string(fromState), string(current.State), Placement{}, now), &copy, nil
}

func (controller *Controller) applyPlacementMessageLocked(envelope Envelope) (ControlEvent, *Worker, *Placement, error) {
	now := controller.now().UTC()
	placement, ok := controller.state.Placements[envelope.PlacementID]
	if !ok {
		return ControlEvent{}, nil, nil, ErrNotFound
	}
	if placement.WorkerID != envelope.WorkerID {
		return ControlEvent{}, nil, nil, fmt.Errorf("%w: placement %s belongs to worker %s", ErrConflict, placement.ID, placement.WorkerID)
	}
	if envelope.Sequence != placement.LastSequence+1 {
		return ControlEvent{}, nil, nil, fmt.Errorf("%w: placement %s expected sequence %d, got %d", ErrOutOfOrder, placement.ID, placement.LastSequence+1, envelope.Sequence)
	}
	worker, ok := controller.state.Workers[placement.WorkerID]
	if !ok {
		return ControlEvent{}, nil, nil, ErrNotFound
	}
	fromState := placement.State
	if err := applyPlacementTransition(envelope, &placement, &worker, now, controller.state.Workers); err != nil {
		return ControlEvent{}, nil, nil, err
	}
	placement.LastSequence = envelope.Sequence
	placement.UpdatedAt = now
	placement.Version++
	worker.LastSeenAt = now
	worker.UpdatedAt = now
	worker.Version++
	controller.state.Placements[placement.ID] = placement
	controller.state.Workers[worker.ID] = worker
	workerCopy := worker
	placementCopy := placement
	return controller.eventForEnvelope(envelope, "placement", string(fromState), string(placement.State), placement, now), &workerCopy, &placementCopy, nil
}

func applyPlacementTransition(envelope Envelope, placement *Placement, worker *Worker, now time.Time, workers map[string]Worker) error {
	switch envelope.Type {
	case MessageProvision:
		if placement.State != PlacementStatePending || worker.State != WorkerStateReady {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload ProvisionPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || !validProtocolIdentifier(payload.EnvironmentID) {
			return fmt.Errorf("%w: invalid provision payload", ErrInvalidEnvelope)
		}
		placement.EnvironmentID = payload.EnvironmentID
		placement.State = PlacementStateProvisioning
	case MessageSync:
		if placement.State != PlacementStateProvisioning {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload SyncPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || (payload.Mode != SyncModeGit && payload.Mode != SyncModeDirectory) || strings.TrimSpace(payload.Digest) == "" {
			return fmt.Errorf("%w: invalid sync payload", ErrInvalidEnvelope)
		}
		placement.Sync.Mode = payload.Mode
		placement.Sync.ManifestDigest = strings.TrimSpace(payload.Digest)
		placement.State = PlacementStateSyncing
	case MessageLease:
		if placement.State != PlacementStateSyncing || worker.State != WorkerStateReady {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload LeasePayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || payload.LeaseTTLMS <= 0 {
			return fmt.Errorf("%w: invalid lease payload", ErrInvalidEnvelope)
		}
		expires := now.Add(time.Duration(payload.LeaseTTLMS) * time.Millisecond)
		placement.LeaseExpiresAt = &expires
		worker.LeaseExpiresAt = &expires
		placement.State = PlacementStateReady
		worker.State = WorkerStateLeased
	case MessageExecute:
		if placement.State != PlacementStateReady && placement.State != PlacementStateCheckpointed && placement.State != PlacementStateRehydrating {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload ExecutePayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || strings.TrimSpace(payload.TaskToken) == "" {
			return fmt.Errorf("%w: task-scoped token is required", ErrInvalidEnvelope)
		}
		if payload.Resume && placement.Checkpoint == nil {
			return fmt.Errorf("%w: resume requires a checkpoint", ErrInvalidTransition)
		}
		placement.State = PlacementStateExecuting
		worker.State = WorkerStateExecuting
	case MessageStream:
		if placement.State != PlacementStateExecuting {
			return invalidPlacementTransition(placement, envelope.Type)
		}
	case MessageCheckpoint:
		if placement.State != PlacementStateExecuting {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload CheckpointPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || !validProtocolIdentifier(payload.ID) || strings.TrimSpace(payload.Digest) == "" {
			return fmt.Errorf("%w: invalid checkpoint payload", ErrInvalidEnvelope)
		}
		placement.Checkpoint = &Checkpoint{ID: payload.ID, Digest: strings.TrimSpace(payload.Digest), URI: strings.TrimSpace(payload.URI), CreatedAt: now}
		placement.State = PlacementStateCheckpointed
		worker.State = WorkerStateLeased
	case MessageCollect:
		var payload CollectPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return err
		}
		if !payload.Complete {
			if placement.State != PlacementStateExecuting && placement.State != PlacementStateCheckpointed {
				return invalidPlacementTransition(placement, envelope.Type)
			}
			placement.State = PlacementStateCollecting
			worker.State = WorkerStateLeased
			break
		}
		if placement.State != PlacementStateCollecting && placement.State != PlacementStateExecuting && placement.State != PlacementStateCheckpointed {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		placement.SnapshotDigest = strings.TrimSpace(payload.SnapshotDigest)
		if payload.Succeeded {
			placement.State = PlacementStateCompleted
		} else {
			placement.State = PlacementStateFailed
		}
		worker.State = WorkerStateLeased
	case MessageLost:
		if placement.State != PlacementStateExecuting && placement.State != PlacementStateCheckpointed && placement.State != PlacementStateCollecting && placement.State != PlacementStateReady {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		placement.State = PlacementStateLost
		worker.State = WorkerStateLost
		worker.LeaseExpiresAt = nil
	case MessageReclaim:
		if placement.State != PlacementStateLost {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		placement.State = PlacementStateReclaiming
	case MessageRehydrate:
		if placement.State != PlacementStateReclaiming {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		var payload RehydratePayload
		if err := decodePayload(envelope.Payload, &payload); err != nil || !validProtocolIdentifier(payload.ReplacementWorkerID) || !validProtocolIdentifier(payload.EnvironmentID) || strings.TrimSpace(payload.SnapshotDigest) == "" {
			return fmt.Errorf("%w: invalid rehydrate payload", ErrInvalidEnvelope)
		}
		replacement, ok := workers[payload.ReplacementWorkerID]
		if !ok || replacement.State != WorkerStateReady || !replacement.Capabilities.Resume {
			return fmt.Errorf("%w: replacement worker %s is unavailable", ErrInvalidTransition, payload.ReplacementWorkerID)
		}
		placement.WorkerID = replacement.ID
		placement.EnvironmentID = payload.EnvironmentID
		placement.SnapshotDigest = payload.SnapshotDigest
		placement.RecoveryCount++
		placement.State = PlacementStateRehydrating
		replacement.State = WorkerStateLeased
		replacement.UpdatedAt = now
		replacement.Version++
		workers[replacement.ID] = replacement
		*worker = replacement
	case MessageDestroy:
		if placement.State != PlacementStateCompleted && placement.State != PlacementStateFailed && placement.State != PlacementStateCollecting && placement.State != PlacementStateLost && placement.State != PlacementStateReclaiming {
			return invalidPlacementTransition(placement, envelope.Type)
		}
		placement.State = PlacementStateDestroyed
		placement.LeaseExpiresAt = nil
		if worker.State != WorkerStateLost && worker.State != WorkerStateDestroyed {
			worker.State = WorkerStateReady
			worker.LeaseExpiresAt = nil
		}
	default:
		return fmt.Errorf("%w: unsupported placement message %s", ErrInvalidEnvelope, envelope.Type)
	}
	return nil
}

func invalidPlacementTransition(placement *Placement, messageType MessageType) error {
	return fmt.Errorf("%w: placement %s cannot apply %s from %s", ErrInvalidTransition, placement.ID, messageType, placement.State)
}

func (controller *Controller) eventForEnvelope(envelope Envelope, entity, fromState, toState string, placement Placement, now time.Time) ControlEvent {
	return ControlEvent{
		ID: envelope.MessageID, MessageID: envelope.MessageID, IdempotencyKey: envelope.IdempotencyKey,
		Type: string(envelope.Type), Entity: entity, WorkerID: envelope.WorkerID,
		PlacementID: envelope.PlacementID, WorkspaceID: placement.WorkspaceID,
		WorkID: placement.WorkID, StepID: placement.StepID, AttemptID: placement.AttemptID,
		Sequence: envelope.Sequence, FromState: fromState, ToState: toState,
		Payload: sanitizedControlPayload(envelope), OccurredAt: now,
	}
}

func sanitizedControlPayload(envelope Envelope) json.RawMessage {
	switch envelope.Type {
	case MessageExecute:
		var payload ExecutePayload
		_ = json.Unmarshal(envelope.Payload, &payload)
		requestDigest := ""
		if len(payload.Request) > 0 {
			digest := sha256.Sum256(payload.Request)
			requestDigest = "sha256:" + hex.EncodeToString(digest[:])
		}
		raw, _ := json.Marshal(map[string]any{
			"resume": payload.Resume, "checkpoint_id": payload.CheckpointID,
			"checkpoint_digest": payload.CheckpointHash, "request_digest": requestDigest,
		})
		return raw
	case MessageStream:
		var payload StreamPayload
		_ = json.Unmarshal(envelope.Payload, &payload)
		digest := sha256.Sum256(envelope.Payload)
		raw, _ := json.Marshal(map[string]any{
			"kind": strings.TrimSpace(payload.Kind), "text_bytes": len(payload.Text),
			"payload_digest": "sha256:" + hex.EncodeToString(digest[:]),
		})
		return raw
	case MessageProvision:
		var payload ProvisionPayload
		_ = json.Unmarshal(envelope.Payload, &payload)
		manifestDigest := ""
		if len(payload.Manifest) > 0 {
			digest := sha256.Sum256(payload.Manifest)
			manifestDigest = "sha256:" + hex.EncodeToString(digest[:])
		}
		raw, _ := json.Marshal(map[string]any{
			"environment_id": payload.EnvironmentID, "root_dir": payload.RootDir,
			"manifest_digest": manifestDigest, "policy": payload.Policy,
		})
		return raw
	case MessageCollect:
		var payload CollectPayload
		_ = json.Unmarshal(envelope.Payload, &payload)
		raw, _ := json.Marshal(map[string]any{
			"complete": payload.Complete, "succeeded": payload.Succeeded,
			"snapshot_digest": payload.SnapshotDigest, "artifact_count": payload.ArtifactCount,
		})
		return raw
	case MessageDestroy:
		var payload DestroyPayload
		_ = json.Unmarshal(envelope.Payload, &payload)
		raw, _ := json.Marshal(map[string]any{"reason": strings.TrimSpace(payload.Reason)})
		return raw
	default:
		return append(json.RawMessage(nil), envelope.Payload...)
	}
}

func (controller *Controller) duplicateResultLocked(ctx context.Context, envelope Envelope) ApplyResult {
	for index := range controller.state.Events {
		if controller.state.Events[index].MessageID != envelope.MessageID && controller.state.Events[index].IdempotencyKey != envelope.IdempotencyKey {
			continue
		}
		if !controller.state.Events[index].Published {
			_ = controller.publishEventLocked(ctx, index)
		}
		result := ApplyResult{Duplicate: true, Event: controller.state.Events[index]}
		if worker, ok := controller.state.Workers[envelope.WorkerID]; ok {
			copy := worker
			result.Worker = &copy
		}
		if placement, ok := controller.state.Placements[envelope.PlacementID]; ok {
			copy := placement
			result.Placement = &copy
		}
		return result
	}
	return ApplyResult{Duplicate: true}
}

func (controller *Controller) appendAndSaveLocked(event ControlEvent) (int, error) {
	controller.state.Events = append(controller.state.Events, event)
	controller.state.UpdatedAt = controller.now().UTC()
	if err := controller.saveLocked(); err != nil {
		controller.state.Events = controller.state.Events[:len(controller.state.Events)-1]
		return -1, err
	}
	return len(controller.state.Events) - 1, nil
}

func (controller *Controller) publishEventLocked(ctx context.Context, index int) error {
	if index < 0 || index >= len(controller.state.Events) || controller.state.Events[index].Published {
		return nil
	}
	if controller.eventSink != nil {
		if err := controller.eventSink.Record(ctx, controller.state.Events[index]); err != nil {
			return fmt.Errorf("workerprotocol: publish control event: %w", err)
		}
	}
	controller.state.Events[index].Published = true
	return controller.saveLocked()
}

func (controller *Controller) saveLocked() error {
	raw, err := json.MarshalIndent(controller.state, "", "  ")
	if err != nil {
		return fmt.Errorf("workerprotocol: encode controller state: %w", err)
	}
	if err := atomicwrite.Write(controller.statePath, append(raw, '\n')); err != nil {
		return fmt.Errorf("workerprotocol: save controller state: %w", err)
	}
	return nil
}

func (controller *Controller) initializeMapsLocked() {
	if controller.state.Workers == nil {
		controller.state.Workers = make(map[string]Worker)
	}
	if controller.state.Placements == nil {
		controller.state.Placements = make(map[string]Placement)
	}
	if controller.state.Events == nil {
		controller.state.Events = make([]ControlEvent, 0)
	}
	if controller.state.AppliedMessages == nil {
		controller.state.AppliedMessages = make(map[string]string)
	}
	if controller.state.AppliedIdempotencyKeys == nil {
		controller.state.AppliedIdempotencyKeys = make(map[string]string)
	}
}

func cloneControllerState(state controllerState) controllerState {
	clone := state
	clone.Workers = make(map[string]Worker, len(state.Workers))
	for id, worker := range state.Workers {
		if worker.LeaseExpiresAt != nil {
			expiresAt := *worker.LeaseExpiresAt
			worker.LeaseExpiresAt = &expiresAt
		}
		clone.Workers[id] = worker
	}
	clone.Placements = make(map[string]Placement, len(state.Placements))
	for id, placement := range state.Placements {
		placement.Policy.Egress.AllowHosts = append([]string(nil), placement.Policy.Egress.AllowHosts...)
		if placement.LeaseExpiresAt != nil {
			expiresAt := *placement.LeaseExpiresAt
			placement.LeaseExpiresAt = &expiresAt
		}
		if placement.Checkpoint != nil {
			checkpoint := *placement.Checkpoint
			placement.Checkpoint = &checkpoint
		}
		clone.Placements[id] = placement
	}
	clone.Events = make([]ControlEvent, len(state.Events))
	for index, event := range state.Events {
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		clone.Events[index] = event
	}
	clone.AppliedMessages = make(map[string]string, len(state.AppliedMessages))
	for messageID, fingerprint := range state.AppliedMessages {
		clone.AppliedMessages[messageID] = fingerprint
	}
	clone.AppliedIdempotencyKeys = make(map[string]string, len(state.AppliedIdempotencyKeys))
	for key, fingerprint := range state.AppliedIdempotencyKeys {
		clone.AppliedIdempotencyKeys[key] = fingerprint
	}
	return clone
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: decode %T: %v", ErrInvalidEnvelope, target, err)
	}
	return nil
}

func envelopeFingerprint(envelope Envelope) string {
	payload := idempotencyPayload(envelope)
	raw, _ := json.Marshal(struct {
		ProtocolVersion string          `json:"protocol_version"`
		Type            MessageType     `json:"type"`
		WorkerID        string          `json:"worker_id"`
		PlacementID     string          `json:"placement_id"`
		Sequence        int64           `json:"sequence"`
		Payload         json.RawMessage `json:"payload"`
	}{envelope.ProtocolVersion, envelope.Type, envelope.WorkerID, envelope.PlacementID, envelope.Sequence, payload})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func idempotencyPayload(envelope Envelope) json.RawMessage {
	switch envelope.Type {
	case MessageExecute:
		var payload ExecutePayload
		if json.Unmarshal(envelope.Payload, &payload) == nil {
			payload.TaskToken = ""
			raw, _ := json.Marshal(payload)
			return raw
		}
	case MessageCollect:
		var payload CollectPayload
		if json.Unmarshal(envelope.Payload, &payload) == nil {
			payload.TaskToken = ""
			raw, _ := json.Marshal(payload)
			return raw
		}
	case MessageDestroy:
		var payload DestroyPayload
		if json.Unmarshal(envelope.Payload, &payload) == nil {
			payload.TaskToken = ""
			raw, _ := json.Marshal(payload)
			return raw
		}
	}
	return append(json.RawMessage(nil), envelope.Payload...)
}
