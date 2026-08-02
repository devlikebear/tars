package workerprotocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type RehydratePlacementInput struct {
	PlacementID         string
	ReplacementWorkerID string
	EnvironmentID       string
	SnapshotDigest      string
	CheckpointID        string
	CheckpointDigest    string
	LeaseTTLMS          int64
}

func (controller *Controller) ReconcileExpired(ctx context.Context, reason string) ([]Placement, error) {
	if controller == nil {
		return nil, fmt.Errorf("workerprotocol: controller is nil")
	}
	snapshot := controller.Snapshot()
	now := controller.now().UTC()
	placementIDs := make([]string, 0, len(snapshot.Placements))
	for placementID := range snapshot.Placements {
		placementIDs = append(placementIDs, placementID)
	}
	sort.Strings(placementIDs)
	lost := make([]Placement, 0)
	workersWithActivePlacement := make(map[string]struct{})
	for _, placementID := range placementIDs {
		placement := snapshot.Placements[placementID]
		if !placementCanBeLost(placement.State) {
			continue
		}
		workersWithActivePlacement[placement.WorkerID] = struct{}{}
		worker := snapshot.Workers[placement.WorkerID]
		placementExpired := placement.LeaseExpiresAt != nil && !placement.LeaseExpiresAt.After(now)
		workerExpired := worker.LeaseExpiresAt != nil && !worker.LeaseExpiresAt.After(now)
		if !placementExpired && !workerExpired && worker.State != WorkerStateLost && worker.State != WorkerStateDisconnected {
			continue
		}
		envelope := controller.recoveryEnvelope(placement.WorkerID, placement.ID, placement.LastSequence+1, MessageLost, LostPayload{Reason: boundedRecoveryReason(reason)})
		result, err := controller.Apply(ctx, envelope)
		if err != nil {
			return lost, err
		}
		if result.Placement != nil {
			lost = append(lost, *result.Placement)
		}
	}

	snapshot = controller.Snapshot()
	workerIDs := make([]string, 0, len(snapshot.Workers))
	for workerID := range snapshot.Workers {
		workerIDs = append(workerIDs, workerID)
	}
	sort.Strings(workerIDs)
	for _, workerID := range workerIDs {
		worker := snapshot.Workers[workerID]
		if _, active := workersWithActivePlacement[workerID]; active || worker.LeaseExpiresAt == nil || worker.LeaseExpiresAt.After(now) || worker.State == WorkerStateLost || worker.State == WorkerStateDestroyed {
			continue
		}
		envelope := controller.recoveryEnvelope(worker.ID, "", worker.LastSequence+1, MessageLost, LostPayload{Reason: boundedRecoveryReason(reason)})
		if _, err := controller.Apply(ctx, envelope); err != nil {
			return lost, err
		}
	}
	return lost, nil
}

func (controller *Controller) BeginReclaim(ctx context.Context, placementID, reason string) (Placement, error) {
	if controller == nil || !validProtocolIdentifier(placementID) {
		return Placement{}, ErrInvalidEnvelope
	}
	placement, ok := controller.Snapshot().Placements[placementID]
	if !ok {
		return Placement{}, ErrNotFound
	}
	envelope := controller.recoveryEnvelope(placement.WorkerID, placement.ID, placement.LastSequence+1, MessageReclaim, ReclaimPayload{Reason: boundedRecoveryReason(reason)})
	result, err := controller.Apply(ctx, envelope)
	if err != nil {
		return Placement{}, err
	}
	if result.Placement == nil {
		return Placement{}, ErrNotFound
	}
	return *result.Placement, nil
}

func (controller *Controller) RehydratePlacement(ctx context.Context, input RehydratePlacementInput) (Placement, error) {
	if controller == nil || !validProtocolIdentifier(input.PlacementID) || !validProtocolIdentifier(input.ReplacementWorkerID) ||
		!validProtocolIdentifier(input.EnvironmentID) || strings.TrimSpace(input.SnapshotDigest) == "" || input.LeaseTTLMS <= 0 {
		return Placement{}, ErrInvalidEnvelope
	}
	placement, ok := controller.Snapshot().Placements[input.PlacementID]
	if !ok {
		return Placement{}, ErrNotFound
	}
	payload := RehydratePayload{
		ReplacementWorkerID: input.ReplacementWorkerID, EnvironmentID: input.EnvironmentID,
		SnapshotDigest: strings.TrimSpace(input.SnapshotDigest), CheckpointID: input.CheckpointID,
		CheckpointDigest: input.CheckpointDigest, LeaseTTLMS: input.LeaseTTLMS,
	}
	envelope := controller.recoveryEnvelope(placement.WorkerID, placement.ID, placement.LastSequence+1, MessageRehydrate, payload)
	result, err := controller.Apply(ctx, envelope)
	if err != nil {
		return Placement{}, err
	}
	if result.Placement == nil {
		return Placement{}, ErrNotFound
	}
	return *result.Placement, nil
}

func (controller *Controller) recoveryEnvelope(workerID, placementID string, sequence int64, messageType MessageType, payload any) Envelope {
	raw, _ := json.Marshal(payload)
	scope := workerID
	if placementID != "" {
		scope = placementID
	}
	messageID := fmt.Sprintf("%s:recovery:%s:%d", scope, messageType, sequence)
	return Envelope{
		ProtocolVersion: ProtocolVersionV1, MessageID: messageID, IdempotencyKey: messageID,
		Type: messageType, WorkerID: workerID, PlacementID: placementID,
		Sequence: sequence, SentAt: controller.now().UTC(), Payload: raw,
	}
}

func placementCanBeLost(state PlacementState) bool {
	switch state {
	case PlacementStateReady, PlacementStateExecuting, PlacementStateCheckpointed, PlacementStateCollecting:
		return true
	default:
		return false
	}
}

func boundedRecoveryReason(reason string) string {
	reason = strings.Join(strings.Fields(strings.TrimSpace(reason)), " ")
	if len(reason) > 512 {
		reason = reason[:512]
	}
	return reason
}
