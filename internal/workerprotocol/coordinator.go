package workerprotocol

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GatewayCoordinatorOptions struct {
	Controller    *Controller
	WorkerID      string
	TransportName string
	Endpoint      string
	Capabilities  WorkerCapabilities
	Transport     WorkerTransport
	TokenIssuer   *TaskTokenIssuer
	Quarantine    *ArtifactQuarantine
	LeaseTTL      time.Duration
	TokenTTL      time.Duration
	Now           func() time.Time
}

type GatewayCoordinator struct {
	mu            sync.Mutex
	controller    *Controller
	workerID      string
	transportName string
	endpoint      string
	capabilities  WorkerCapabilities
	transport     WorkerTransport
	tokenIssuer   *TaskTokenIssuer
	quarantine    *ArtifactQuarantine
	leaseTTL      time.Duration
	tokenTTL      time.Duration
	now           func() time.Time
}

type RemoteRunInput struct {
	PlacementID   string
	EnvironmentID string
	WorkspaceID   string
	WorkID        string
	StepID        string
	AttemptID     string
	Policy        ExecutionPolicy
	Workspace     WorkspaceBundle
	Request       json.RawMessage
	RedactValues  []string
}

type RemoteRunResult struct {
	Succeeded         bool               `json:"succeeded"`
	Payload           json.RawMessage    `json:"payload,omitempty"`
	Artifacts         []ReleasedArtifact `json:"artifacts,omitempty"`
	RejectedArtifacts []RejectedArtifact `json:"rejected_artifacts,omitempty"`
	Checkpoint        *CheckpointPayload `json:"checkpoint,omitempty"`
}

type RemoteRecoveryInput struct {
	PlacementID   string
	EnvironmentID string
	Workspace     WorkspaceBundle
	Request       json.RawMessage
	RedactValues  []string
	Reason        string
}

func NewGatewayCoordinator(opts GatewayCoordinatorOptions) (*GatewayCoordinator, error) {
	if opts.Controller == nil || opts.Transport == nil || opts.TokenIssuer == nil || opts.Quarantine == nil ||
		!validProtocolIdentifier(opts.WorkerID) || strings.TrimSpace(opts.TransportName) == "" || strings.TrimSpace(opts.Endpoint) == "" {
		return nil, fmt.Errorf("workerprotocol: gateway coordinator dependencies and worker identity are required")
	}
	if !opts.Capabilities.EgressPolicy || !opts.Capabilities.ResourceLimits {
		return nil, fmt.Errorf("%w: remote worker must enforce egress and resource limits", ErrInvalidPolicy)
	}
	if opts.LeaseTTL <= 0 || opts.TokenTTL <= 0 || opts.TokenTTL > opts.LeaseTTL {
		return nil, fmt.Errorf("%w: task token TTL must fit within the worker lease", ErrTaskTokenTTL)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &GatewayCoordinator{
		controller: opts.Controller, workerID: strings.TrimSpace(opts.WorkerID),
		transportName: strings.TrimSpace(opts.TransportName), endpoint: strings.TrimSpace(opts.Endpoint),
		capabilities: opts.Capabilities, transport: opts.Transport, tokenIssuer: opts.TokenIssuer,
		quarantine: opts.Quarantine, leaseTTL: opts.LeaseTTL, tokenTTL: opts.TokenTTL, now: opts.Now,
	}, nil
}

func (coordinator *GatewayCoordinator) Run(ctx context.Context, input RemoteRunInput) (RemoteRunResult, error) {
	if coordinator == nil {
		return RemoteRunResult{}, fmt.Errorf("workerprotocol: gateway coordinator is not configured")
	}
	if err := validateRemoteRunInput(input); err != nil {
		return RemoteRunResult{}, err
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := coordinator.controller.FlushPending(ctx); err != nil {
		return RemoteRunResult{}, err
	}
	if err := coordinator.ensureWorkerReady(ctx); err != nil {
		return RemoteRunResult{}, err
	}
	if existing, ok := coordinator.controller.Snapshot().Placements[input.PlacementID]; ok {
		return RemoteRunResult{}, fmt.Errorf("%w: placement %s already exists in state %s", ErrConflict, existing.ID, existing.State)
	}
	if _, err := coordinator.controller.CreatePlacement(ctx, CreatePlacementInput{
		ID: input.PlacementID, WorkspaceID: input.WorkspaceID, WorkID: input.WorkID,
		StepID: input.StepID, AttemptID: input.AttemptID, WorkerID: coordinator.workerID,
		Policy: input.Policy, Sync: SyncSpec{
			Mode: input.Workspace.Manifest.Mode, SourceOwner: OwnerGateway,
			WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway,
			ManifestDigest: input.Workspace.Manifest.Digest,
		},
	}); err != nil {
		return RemoteRunResult{}, err
	}
	binding := TaskTokenBinding{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
		AttemptID: input.AttemptID, PlacementID: input.PlacementID, WorkerID: coordinator.workerID,
	}
	sequence := int64(1)
	if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageProvision, ProvisionPayload{
		EnvironmentID: input.EnvironmentID, Policy: input.Policy, Binding: binding,
	}), nil); err != nil {
		return RemoteRunResult{}, err
	}
	sequence++
	if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageSync, SyncPayload{
		Mode: input.Workspace.Manifest.Mode, Digest: input.Workspace.Manifest.Digest,
	}), &input.Workspace); err != nil {
		return RemoteRunResult{}, err
	}
	sequence++
	leaseTTLMS := coordinator.leaseTTL.Milliseconds()
	if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageLease, LeasePayload{LeaseTTLMS: leaseTTLMS}), nil); err != nil {
		return RemoteRunResult{}, err
	}
	token, _, err := coordinator.tokenIssuer.Issue(binding, []TaskScope{
		TaskScopeExecute, TaskScopeStream, TaskScopeCheckpoint, TaskScopeCollect, TaskScopeDestroy,
	}, coordinator.tokenTTL)
	if err != nil {
		return RemoteRunResult{}, err
	}
	sequence++
	executeResponse, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageExecute, ExecutePayload{
		TaskToken: token, Request: append(json.RawMessage(nil), input.Request...),
	}), nil)
	if err != nil {
		return RemoteRunResult{}, err
	}
	return coordinator.finalizeExecution(ctx, input, token, sequence, executeResponse)
}

func (coordinator *GatewayCoordinator) Resume(ctx context.Context, input RemoteRecoveryInput) (RemoteRunResult, error) {
	if coordinator == nil || !validProtocolIdentifier(input.PlacementID) || !validProtocolIdentifier(input.EnvironmentID) {
		return RemoteRunResult{}, ErrInvalidEnvelope
	}
	if err := VerifyWorkspaceBundle(input.Workspace, DefaultWorkspaceBundleLimits()); err != nil {
		return RemoteRunResult{}, err
	}
	if len(input.Request) > 0 && !json.Valid(input.Request) {
		return RemoteRunResult{}, ErrInvalidEnvelope
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := coordinator.controller.FlushPending(ctx); err != nil {
		return RemoteRunResult{}, err
	}
	if err := coordinator.ensureWorkerReady(ctx); err != nil {
		return RemoteRunResult{}, err
	}
	placement, ok := coordinator.controller.Snapshot().Placements[input.PlacementID]
	if !ok {
		return RemoteRunResult{}, ErrNotFound
	}
	if placement.State != PlacementStateLost {
		return RemoteRunResult{}, fmt.Errorf("%w: placement %s is %s", ErrInvalidTransition, placement.ID, placement.State)
	}
	reclaiming, err := coordinator.controller.BeginReclaim(ctx, placement.ID, input.Reason)
	if err != nil {
		return RemoteRunResult{}, err
	}
	binding := TaskTokenBinding{
		WorkspaceID: placement.WorkspaceID, WorkID: placement.WorkID, StepID: placement.StepID,
		AttemptID: placement.AttemptID, PlacementID: placement.ID, WorkerID: coordinator.workerID,
	}
	rehydratePayload := RehydratePayload{
		ReplacementWorkerID: coordinator.workerID, EnvironmentID: input.EnvironmentID,
		SnapshotDigest: input.Workspace.Manifest.Digest, LeaseTTLMS: coordinator.leaseTTL.Milliseconds(),
		Binding: binding, Policy: placement.Policy,
	}
	if placement.Checkpoint != nil {
		rehydratePayload.CheckpointID = placement.Checkpoint.ID
		rehydratePayload.CheckpointDigest = placement.Checkpoint.Digest
	}
	sequence := reclaiming.LastSequence + 1
	if _, err := coordinator.exchangeOnly(ctx, coordinator.envelope(placement.ID, sequence, MessageRehydrate, rehydratePayload), &input.Workspace); err != nil {
		return RemoteRunResult{}, err
	}
	rehydrated, err := coordinator.controller.RehydratePlacement(ctx, RehydratePlacementInput{
		PlacementID: placement.ID, ReplacementWorkerID: coordinator.workerID,
		EnvironmentID: input.EnvironmentID, SnapshotDigest: input.Workspace.Manifest.Digest,
		CheckpointID: rehydratePayload.CheckpointID, CheckpointDigest: rehydratePayload.CheckpointDigest,
		LeaseTTLMS: rehydratePayload.LeaseTTLMS,
	})
	if err != nil {
		return RemoteRunResult{}, err
	}
	if rehydrated.LastSequence != sequence || rehydrated.WorkerID != coordinator.workerID {
		return RemoteRunResult{}, fmt.Errorf("%w: rehydrated placement sequence diverged", ErrConflict)
	}
	token, _, err := coordinator.tokenIssuer.Issue(binding, []TaskScope{
		TaskScopeExecute, TaskScopeStream, TaskScopeCheckpoint, TaskScopeCollect, TaskScopeDestroy,
	}, coordinator.tokenTTL)
	if err != nil {
		return RemoteRunResult{}, err
	}
	sequence++
	executeResponse, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(placement.ID, sequence, MessageExecute, ExecutePayload{
		TaskToken: token, Resume: true, CheckpointID: rehydratePayload.CheckpointID,
		CheckpointHash: rehydratePayload.CheckpointDigest, Request: append(json.RawMessage(nil), input.Request...),
	}), nil)
	if err != nil {
		return RemoteRunResult{}, err
	}
	runInput := RemoteRunInput{
		PlacementID: placement.ID, EnvironmentID: input.EnvironmentID,
		WorkspaceID: placement.WorkspaceID, WorkID: placement.WorkID, StepID: placement.StepID, AttemptID: placement.AttemptID,
		Policy: placement.Policy, Workspace: input.Workspace, Request: input.Request, RedactValues: input.RedactValues,
	}
	return coordinator.finalizeExecution(ctx, runInput, token, sequence, executeResponse)
}

func (coordinator *GatewayCoordinator) finalizeExecution(ctx context.Context, input RemoteRunInput, token string, sequence int64, executeResponse WireResponse) (RemoteRunResult, error) {
	result := RemoteRunResult{
		Payload:    append(json.RawMessage(nil), executeResponse.Payload...),
		Checkpoint: cloneCheckpointPayload(executeResponse.Checkpoint),
	}
	succeeded, err := remoteResponseSucceeded(executeResponse.Payload)
	if err != nil {
		return RemoteRunResult{}, err
	}
	result.Succeeded = succeeded
	if executeResponse.Checkpoint != nil {
		sequence++
		checkpoint := *executeResponse.Checkpoint
		if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageCheckpoint, checkpoint), nil); err != nil {
			return RemoteRunResult{}, err
		}
	}
	sequence++
	_, err = coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageCollect, CollectPayload{
		Complete: false, TaskToken: token,
	}), nil)
	if err != nil {
		return RemoteRunResult{}, err
	}
	quarantineReport, err := coordinator.quarantine.InspectAndRelease(ctx, input.PlacementID, executeResponse.Artifacts, input.RedactValues)
	if err != nil {
		return RemoteRunResult{}, err
	}
	result.Artifacts = quarantineReport.Accepted
	result.RejectedArtifacts = quarantineReport.Rejected
	if len(result.RejectedArtifacts) != 0 {
		result.Succeeded = false
	}
	sequence++
	snapshotDigest := digestBytes(result.Payload)
	if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageCollect, CollectPayload{
		Complete: true, Succeeded: result.Succeeded, SnapshotDigest: snapshotDigest,
		ArtifactCount: len(result.Artifacts), TaskToken: token,
	}), nil); err != nil {
		return RemoteRunResult{}, err
	}
	sequence++
	if _, err := coordinator.exchangeAndApply(ctx, coordinator.envelope(input.PlacementID, sequence, MessageDestroy, DestroyPayload{
		Reason: "remote execution finalized", TaskToken: token,
	}), nil); err != nil {
		return RemoteRunResult{}, err
	}
	return result, coordinator.controller.FlushPending(ctx)
}

func (coordinator *GatewayCoordinator) ensureWorkerReady(ctx context.Context) error {
	snapshot := coordinator.controller.Snapshot()
	worker, exists := snapshot.Workers[coordinator.workerID]
	if exists && worker.State == WorkerStateReady {
		return nil
	}
	if !exists || worker.State == WorkerStateLost || worker.State == WorkerStateDisconnected || worker.State == WorkerStateDestroyed {
		sequence := int64(1)
		if exists {
			sequence = worker.LastSequence + 1
		}
		response, err := coordinator.exchangeAndApply(ctx, coordinator.envelope("", sequence, MessageRegister, RegisterPayload{
			Transport: coordinator.transportName, Endpoint: coordinator.endpoint, Capabilities: coordinator.capabilities,
		}), nil)
		if err != nil {
			return err
		}
		var registration RegisterPayload
		if err := json.Unmarshal(response.Payload, &registration); err != nil || registration.Capabilities != coordinator.capabilities {
			return fmt.Errorf("%w: remote worker capability attestation changed", ErrTransportConfig)
		}
		worker = coordinator.controller.Snapshot().Workers[coordinator.workerID]
	}
	if worker.State != WorkerStateRegistered {
		return fmt.Errorf("%w: worker %s is %s", ErrInvalidTransition, worker.ID, worker.State)
	}
	_, err := coordinator.exchangeAndApply(ctx, coordinator.envelope("", worker.LastSequence+1, MessageHeartbeat, HeartbeatPayload{}), nil)
	return err
}

func (coordinator *GatewayCoordinator) exchangeAndApply(ctx context.Context, envelope Envelope, workspace *WorkspaceBundle) (WireResponse, error) {
	response, err := coordinator.exchangeOnly(ctx, envelope, workspace)
	if err != nil {
		return WireResponse{}, err
	}
	if _, err := coordinator.controller.Apply(ctx, envelope); err != nil {
		return WireResponse{}, err
	}
	if err := coordinator.controller.FlushPending(ctx); err != nil {
		return WireResponse{}, err
	}
	return response, nil
}

func (coordinator *GatewayCoordinator) exchangeOnly(ctx context.Context, envelope Envelope, workspace *WorkspaceBundle) (WireResponse, error) {
	request := WireRequest{ProtocolVersion: ProtocolVersionV1, RequestID: envelope.MessageID, Envelope: envelope, Workspace: workspace}
	response, err := coordinator.transport.Exchange(ctx, request)
	if err != nil {
		return WireResponse{}, err
	}
	if !response.Accepted {
		return WireResponse{}, fmt.Errorf("workerprotocol: remote worker rejected %s: %s", envelope.Type, response.ErrorCode)
	}
	return response, nil
}

func (coordinator *GatewayCoordinator) envelope(placementID string, sequence int64, messageType MessageType, payload any) Envelope {
	raw, _ := json.Marshal(payload)
	scope := coordinator.workerID
	if placementID != "" {
		scope = placementID
	}
	messageID := scope + ":control:" + messageType.String() + ":" + strconv.FormatInt(sequence, 10)
	return Envelope{
		ProtocolVersion: ProtocolVersionV1, MessageID: messageID,
		IdempotencyKey: messageID, Type: messageType, WorkerID: coordinator.workerID,
		PlacementID: placementID, Sequence: sequence, SentAt: coordinator.now().UTC(), Payload: raw,
	}
}

func validateRemoteRunInput(input RemoteRunInput) error {
	if !validProtocolIdentifier(input.PlacementID) || !validProtocolIdentifier(input.EnvironmentID) ||
		!validProtocolIdentifier(input.WorkspaceID) || !validProtocolIdentifier(input.WorkID) ||
		!validProtocolIdentifier(input.StepID) || !validProtocolIdentifier(input.AttemptID) {
		return ErrInvalidEnvelope
	}
	if err := input.Policy.Validate(); err != nil {
		return err
	}
	if err := VerifyWorkspaceBundle(input.Workspace, DefaultWorkspaceBundleLimits()); err != nil {
		return err
	}
	if len(input.Request) > 0 && !json.Valid(input.Request) {
		return ErrInvalidEnvelope
	}
	return nil
}

func remoteResponseSucceeded(payload json.RawMessage) (bool, error) {
	var outcome struct {
		Succeeded *bool `json:"succeeded"`
	}
	if err := json.Unmarshal(payload, &outcome); err != nil || outcome.Succeeded == nil {
		return false, fmt.Errorf("%w: remote execution response must report succeeded", ErrWireContract)
	}
	return *outcome.Succeeded, nil
}

func cloneCheckpointPayload(checkpoint *CheckpointPayload) *CheckpointPayload {
	if checkpoint == nil {
		return nil
	}
	copy := *checkpoint
	copy.Metadata = append(json.RawMessage(nil), checkpoint.Metadata...)
	return &copy
}
