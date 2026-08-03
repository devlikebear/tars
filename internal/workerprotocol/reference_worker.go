package workerprotocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const referenceWorkerSchemaVersion = 1

type ReferenceExecutionRequest struct {
	Binding    TaskTokenBinding `json:"binding"`
	RootDir    string           `json:"root_dir"`
	Policy     ExecutionPolicy  `json:"policy"`
	Request    json.RawMessage  `json:"request,omitempty"`
	Resume     bool             `json:"resume,omitempty"`
	Checkpoint *Checkpoint      `json:"checkpoint,omitempty"`
}

type ReferenceExecutionResult struct {
	Payload    json.RawMessage    `json:"payload,omitempty"`
	Artifacts  []WireArtifact     `json:"artifacts,omitempty"`
	Checkpoint *CheckpointPayload `json:"checkpoint,omitempty"`
}

type ReferenceExecutor interface {
	Capabilities() WorkerCapabilities
	Execute(context.Context, ReferenceExecutionRequest) (ReferenceExecutionResult, error)
}

type ReferenceWorkerOptions struct {
	WorkerID      string
	RootDir       string
	StatePath     string
	TokenVerifier *TaskTokenVerifier
	Executor      ReferenceExecutor
	Now           func() time.Time
}

type ReferenceEnvironment struct {
	EnvironmentID  string           `json:"environment_id"`
	PlacementID    string           `json:"placement_id"`
	Binding        TaskTokenBinding `json:"binding"`
	RootDir        string           `json:"root_dir"`
	Policy         ExecutionPolicy  `json:"policy"`
	ManifestDigest string           `json:"manifest_digest,omitempty"`
	State          PlacementState   `json:"state"`
	LastSequence   int64            `json:"last_sequence"`
	LeaseExpiresAt *time.Time       `json:"lease_expires_at,omitempty"`
	Checkpoint     *Checkpoint      `json:"checkpoint,omitempty"`
	ResultDigest   string           `json:"result_digest,omitempty"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type ReferenceWorkerSnapshot struct {
	WorkerID     string                          `json:"worker_id"`
	Capabilities WorkerCapabilities              `json:"capabilities"`
	Environments map[string]ReferenceEnvironment `json:"environments"`
}

type referenceApplied struct {
	Fingerprint string       `json:"fingerprint"`
	Type        MessageType  `json:"type"`
	PlacementID string       `json:"placement_id,omitempty"`
	Response    WireResponse `json:"-"`
}

type referenceWorkerState struct {
	SchemaVersion int                             `json:"schema_version"`
	WorkerID      string                          `json:"worker_id"`
	Environments  map[string]ReferenceEnvironment `json:"environments"`
	AppliedIDs    map[string]referenceApplied     `json:"applied_ids"`
	AppliedKeys   map[string]referenceApplied     `json:"applied_keys"`
	UpdatedAt     time.Time                       `json:"updated_at"`
}

type ReferenceWorker struct {
	mu           sync.Mutex
	workerID     string
	rootDir      string
	placements   string
	statePath    string
	verifier     *TaskTokenVerifier
	executor     ReferenceExecutor
	capabilities WorkerCapabilities
	now          func() time.Time
	environments map[string]ReferenceEnvironment
	results      map[string]ReferenceExecutionResult
	appliedIDs   map[string]referenceApplied
	appliedKeys  map[string]referenceApplied
}

func NewReferenceWorker(opts ReferenceWorkerOptions) (*ReferenceWorker, error) {
	workerID := strings.TrimSpace(opts.WorkerID)
	if !validProtocolIdentifier(workerID) || opts.TokenVerifier == nil || opts.Executor == nil {
		return nil, fmt.Errorf("workerprotocol: reference worker identity, verifier, and executor are required")
	}
	capabilities := opts.Executor.Capabilities()
	if !capabilities.EgressPolicy || !capabilities.ResourceLimits {
		return nil, fmt.Errorf("%w: reference executor must enforce egress and resource limits", ErrInvalidPolicy)
	}
	root := strings.TrimSpace(opts.RootDir)
	if root == "" {
		return nil, fmt.Errorf("workerprotocol: reference worker root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve reference worker root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: create reference worker root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve reference worker root: %w", err)
	}
	placements := filepath.Join(canonical, "placements")
	if err := os.MkdirAll(placements, 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: create reference worker placements: %w", err)
	}
	for _, path := range []string{canonical, placements} {
		if err := os.Chmod(path, 0o700); err != nil {
			return nil, fmt.Errorf("workerprotocol: secure reference worker root: %w", err)
		}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	statePath := strings.TrimSpace(opts.StatePath)
	if statePath == "" {
		statePath = filepath.Join(canonical, "worker-state.json")
	}
	statePath, err = filepath.Abs(statePath)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: invalid reference worker state path")
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: create reference worker state directory: %w", err)
	}
	canonicalStateDir, err := filepath.EvalSymlinks(filepath.Dir(statePath))
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve reference worker state directory: %w", err)
	}
	statePath = filepath.Join(canonicalStateDir, filepath.Base(statePath))
	if filepath.Clean(statePath) == filepath.Clean(canonical) || sameOrWithinWorkspace(placements, statePath) {
		return nil, fmt.Errorf("workerprotocol: invalid reference worker state path")
	}
	if info, statErr := os.Lstat(statePath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workerprotocol: reference worker state path must not be a symlink")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("workerprotocol: inspect reference worker state path: %w", statErr)
	}
	worker := &ReferenceWorker{
		workerID: workerID, rootDir: canonical, placements: placements, verifier: opts.TokenVerifier,
		statePath: statePath,
		executor:  opts.Executor, capabilities: capabilities, now: opts.Now,
		environments: make(map[string]ReferenceEnvironment), results: make(map[string]ReferenceExecutionResult),
		appliedIDs: make(map[string]referenceApplied), appliedKeys: make(map[string]referenceApplied),
	}
	if err := worker.loadState(); err != nil {
		return nil, err
	}
	return worker, nil
}

func (worker *ReferenceWorker) VerificationKeyID() string {
	if worker == nil || worker.verifier == nil {
		return ""
	}
	return worker.verifier.KeyID
}

func (worker *ReferenceWorker) MaxTaskTokenTTL() time.Duration {
	if worker == nil || worker.verifier == nil {
		return 0
	}
	return worker.verifier.maxTTL
}

func (worker *ReferenceWorker) RootDir() string {
	if worker == nil {
		return ""
	}
	return worker.rootDir
}

func (worker *ReferenceWorker) Handle(ctx context.Context, request WireRequest) (WireResponse, error) {
	if worker == nil || worker.executor == nil || worker.verifier == nil {
		return WireResponse{}, fmt.Errorf("workerprotocol: reference worker is not configured")
	}
	if err := request.Validate(); err != nil {
		return WireResponse{}, err
	}
	if request.Envelope.WorkerID != worker.workerID {
		return worker.rejected(request, "worker_mismatch", "request targets a different worker"), nil
	}
	fingerprint := wireRequestFingerprint(request)

	worker.mu.Lock()
	defer worker.mu.Unlock()
	if applied, ok := worker.appliedIDs[request.RequestID]; ok {
		if applied.Fingerprint != fingerprint {
			return worker.rejected(request, "idempotency_conflict", "request id changed content"), nil
		}
		if err := worker.authorizeReplayLocked(request, applied); err != nil {
			return worker.rejected(request, "authorization_denied", "replay authorization was denied"), nil
		}
		return worker.replayLocked(request, applied)
	}
	if applied, ok := worker.appliedKeys[request.Envelope.IdempotencyKey]; ok {
		if applied.Fingerprint != fingerprint {
			return worker.rejected(request, "idempotency_conflict", "idempotency key changed content"), nil
		}
		if err := worker.authorizeReplayLocked(request, applied); err != nil {
			return worker.rejected(request, "authorization_denied", "replay authorization was denied"), nil
		}
		return worker.replayLocked(request, applied)
	}

	response := worker.handleLocked(ctx, request)
	if response.Accepted {
		applied := referenceApplied{
			Fingerprint: fingerprint, Type: request.Envelope.Type,
			PlacementID: request.Envelope.PlacementID, Response: cloneWireResponse(response),
		}
		worker.appliedIDs[request.RequestID] = applied
		worker.appliedKeys[request.Envelope.IdempotencyKey] = applied
		if err := worker.saveStateLocked(); err != nil {
			return WireResponse{}, err
		}
	}
	return response, nil
}

func (worker *ReferenceWorker) authorizeReplayLocked(request WireRequest, applied referenceApplied) error {
	var token string
	var scope TaskScope
	switch applied.Type {
	case MessageExecute:
		var payload ExecutePayload
		if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
			return err
		}
		token, scope = payload.TaskToken, TaskScopeExecute
	case MessageCollect:
		var payload CollectPayload
		if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
			return err
		}
		token, scope = payload.TaskToken, TaskScopeCollect
	case MessageDestroy:
		var payload DestroyPayload
		if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
			return err
		}
		token, scope = payload.TaskToken, TaskScopeDestroy
	default:
		return nil
	}
	environment, ok := worker.environments[applied.PlacementID]
	if !ok {
		return ErrNotFound
	}
	_, err := worker.verifier.Verify(token, environment.Binding, scope)
	return err
}

func (worker *ReferenceWorker) handleLocked(ctx context.Context, request WireRequest) WireResponse {
	envelope := request.Envelope
	if envelope.PlacementID == "" {
		switch envelope.Type {
		case MessageRegister:
			payload, _ := json.Marshal(RegisterPayload{
				Transport: "in-process", Endpoint: "local://" + worker.workerID, Capabilities: worker.capabilities,
				VerificationKeyID: worker.VerificationKeyID(),
			})
			return worker.accepted(request, payload, nil)
		case MessageHeartbeat:
			return worker.accepted(request, json.RawMessage(`{"ready":true}`), nil)
		default:
			return worker.rejected(request, "placement_required", "operation requires a placement")
		}
	}
	environment, exists := worker.environments[envelope.PlacementID]
	if exists && envelope.Sequence != environment.LastSequence+1 {
		return worker.rejected(request, "out_of_order", "placement sequence is out of order")
	}
	if !exists && !((envelope.Type == MessageProvision && envelope.Sequence == 1) || envelope.Type == MessageRehydrate) {
		return worker.rejected(request, "environment_not_found", "placement environment is unavailable")
	}

	var response WireResponse
	switch envelope.Type {
	case MessageProvision:
		response, environment = worker.provisionLocked(request, environment, exists)
	case MessageRehydrate:
		response, environment = worker.rehydrateLocked(ctx, request, environment, exists)
	case MessageSync:
		response, environment = worker.syncLocked(ctx, request, environment)
	case MessageLease:
		response, environment = worker.leaseLocked(request, environment)
	case MessageExecute:
		response, environment = worker.executeLocked(ctx, request, environment)
	case MessageCheckpoint:
		response, environment = worker.checkpointLocked(request, environment)
	case MessageCollect:
		response, environment = worker.collectLocked(request, environment)
	case MessageDestroy:
		response, environment = worker.destroyLocked(request, environment)
	default:
		response = worker.rejected(request, "unsupported_operation", "operation is unsupported by reference worker")
	}
	if response.Accepted {
		environment.LastSequence = envelope.Sequence
		environment.UpdatedAt = worker.now().UTC()
		worker.environments[envelope.PlacementID] = environment
	}
	return response
}

func (worker *ReferenceWorker) provisionLocked(request WireRequest, _ ReferenceEnvironment, exists bool) (WireResponse, ReferenceEnvironment) {
	if exists {
		return worker.rejected(request, "invalid_transition", "placement is already provisioned"), ReferenceEnvironment{}
	}
	var payload ProvisionPayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil || !validProtocolIdentifier(payload.EnvironmentID) || payload.Binding.PlacementID != request.Envelope.PlacementID || payload.Binding.WorkerID != worker.workerID {
		return worker.rejected(request, "invalid_provision", "provision binding or environment is invalid"), ReferenceEnvironment{}
	}
	if err := validateTaskTokenBinding(payload.Binding); err != nil {
		return worker.rejected(request, "invalid_provision", "provision binding is invalid"), ReferenceEnvironment{}
	}
	if err := payload.Policy.Validate(); err != nil {
		return worker.rejected(request, "policy_denied", "execution policy cannot be enforced"), ReferenceEnvironment{}
	}
	root := filepath.Join(worker.placements, request.Envelope.PlacementID)
	if !sameOrWithinWorkspace(worker.placements, root) {
		return worker.rejected(request, "invalid_provision", "placement path is invalid"), ReferenceEnvironment{}
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		return worker.rejected(request, "invalid_transition", "placement root is not empty"), ReferenceEnvironment{}
	} else if err != nil && !os.IsNotExist(err) {
		return worker.rejected(request, "provision_failed", "placement root is unavailable"), ReferenceEnvironment{}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return worker.rejected(request, "provision_failed", "placement root could not be created"), ReferenceEnvironment{}
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return worker.rejected(request, "provision_failed", "placement root could not be secured"), ReferenceEnvironment{}
	}
	environment := ReferenceEnvironment{
		EnvironmentID: payload.EnvironmentID, PlacementID: request.Envelope.PlacementID,
		Binding: payload.Binding, RootDir: root, Policy: payload.Policy,
		State: PlacementStateProvisioning, UpdatedAt: worker.now().UTC(),
	}
	payloadJSON, _ := json.Marshal(map[string]any{
		"environment_id": environment.EnvironmentID, "egress_enforced": worker.capabilities.EgressPolicy,
		"resource_limits_enforced": worker.capabilities.ResourceLimits,
	})
	return worker.accepted(request, payloadJSON, nil), environment
}

func (worker *ReferenceWorker) syncLocked(ctx context.Context, request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	if environment.State != PlacementStateProvisioning || request.Workspace == nil {
		return worker.rejected(request, "invalid_transition", "placement is not ready for workspace sync"), environment
	}
	if err := ApplyWorkspaceBundle(ctx, environment.RootDir, *request.Workspace, DefaultWorkspaceBundleLimits()); err != nil {
		return worker.rejected(request, "sync_failed", "workspace bundle could not be verified and applied"), environment
	}
	environment.ManifestDigest = request.Workspace.Manifest.Digest
	environment.State = PlacementStateReady
	payload, _ := json.Marshal(map[string]any{"manifest_digest": environment.ManifestDigest, "file_count": request.Workspace.Manifest.FileCount})
	return worker.accepted(request, payload, nil), environment
}

func (worker *ReferenceWorker) rehydrateLocked(ctx context.Context, request WireRequest, _ ReferenceEnvironment, exists bool) (WireResponse, ReferenceEnvironment) {
	if exists || request.Workspace == nil {
		return worker.rejected(request, "invalid_transition", "replacement placement already exists or has no snapshot"), ReferenceEnvironment{}
	}
	var payload RehydratePayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil || payload.ReplacementWorkerID != worker.workerID ||
		!validProtocolIdentifier(payload.EnvironmentID) || payload.SnapshotDigest != request.Workspace.Manifest.Digest ||
		payload.Binding.PlacementID != request.Envelope.PlacementID || payload.Binding.WorkerID != worker.workerID || payload.LeaseTTLMS <= 0 {
		return worker.rejected(request, "invalid_rehydrate", "recovery binding or snapshot is invalid"), ReferenceEnvironment{}
	}
	if err := validateTaskTokenBinding(payload.Binding); err != nil {
		return worker.rejected(request, "invalid_rehydrate", "recovery task binding is invalid"), ReferenceEnvironment{}
	}
	if err := payload.Policy.Validate(); err != nil {
		return worker.rejected(request, "policy_denied", "recovery policy cannot be enforced"), ReferenceEnvironment{}
	}
	if (payload.CheckpointID == "") != (payload.CheckpointDigest == "") ||
		(payload.CheckpointID != "" && !validProtocolIdentifier(payload.CheckpointID)) {
		return worker.rejected(request, "checkpoint_mismatch", "recovery checkpoint is invalid"), ReferenceEnvironment{}
	}
	root := filepath.Join(worker.placements, request.Envelope.PlacementID)
	if !sameOrWithinWorkspace(worker.placements, root) {
		return worker.rejected(request, "invalid_rehydrate", "replacement path is unsafe"), ReferenceEnvironment{}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return worker.rejected(request, "rehydrate_failed", "replacement root could not be created"), ReferenceEnvironment{}
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return worker.rejected(request, "rehydrate_failed", "replacement root could not be secured"), ReferenceEnvironment{}
	}
	if err := ApplyWorkspaceBundle(ctx, root, *request.Workspace, DefaultWorkspaceBundleLimits()); err != nil {
		return worker.rejected(request, "rehydrate_failed", "recovery snapshot could not be applied"), ReferenceEnvironment{}
	}
	expiresAt := worker.now().UTC().Add(time.Duration(payload.LeaseTTLMS) * time.Millisecond)
	environment := ReferenceEnvironment{
		EnvironmentID: payload.EnvironmentID, PlacementID: request.Envelope.PlacementID,
		Binding: payload.Binding, RootDir: root, Policy: payload.Policy,
		ManifestDigest: payload.SnapshotDigest, State: PlacementStateRehydrating,
		LeaseExpiresAt: &expiresAt, UpdatedAt: worker.now().UTC(),
	}
	if payload.CheckpointID != "" {
		environment.Checkpoint = &Checkpoint{ID: payload.CheckpointID, Digest: payload.CheckpointDigest, CreatedAt: worker.now().UTC()}
	}
	responsePayload, _ := json.Marshal(map[string]any{
		"environment_id": payload.EnvironmentID, "snapshot_digest": payload.SnapshotDigest,
		"checkpoint_id": payload.CheckpointID, "lease_expires_at": expiresAt,
	})
	return worker.accepted(request, responsePayload, nil), environment
}

func (worker *ReferenceWorker) leaseLocked(request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	if environment.State != PlacementStateReady {
		return worker.rejected(request, "invalid_transition", "placement is not ready to lease"), environment
	}
	var payload LeasePayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil || payload.LeaseTTLMS <= 0 {
		return worker.rejected(request, "invalid_lease", "lease TTL is invalid"), environment
	}
	expiresAt := worker.now().UTC().Add(time.Duration(payload.LeaseTTLMS) * time.Millisecond)
	environment.LeaseExpiresAt = &expiresAt
	environment.State = PlacementStateReady
	responsePayload, _ := json.Marshal(map[string]any{"lease_expires_at": expiresAt})
	return worker.accepted(request, responsePayload, nil), environment
}

func (worker *ReferenceWorker) executeLocked(ctx context.Context, request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	if environment.State != PlacementStateReady && environment.State != PlacementStateCheckpointed && environment.State != PlacementStateRehydrating {
		return worker.rejected(request, "invalid_transition", "placement is not executable"), environment
	}
	if environment.LeaseExpiresAt == nil || !environment.LeaseExpiresAt.After(worker.now().UTC()) {
		return worker.rejected(request, "lease_expired", "placement lease has expired"), environment
	}
	var payload ExecutePayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
		return worker.rejected(request, "invalid_execute", "execution request is invalid"), environment
	}
	if _, err := worker.verifier.Verify(payload.TaskToken, environment.Binding, TaskScopeExecute); err != nil {
		return worker.rejected(request, "authorization_denied", "task authorization was denied"), environment
	}
	if payload.Resume && (environment.Checkpoint == nil || !worker.capabilities.Resume) {
		return worker.rejected(request, "resume_unavailable", "placement cannot resume"), environment
	}
	result, err := worker.executor.Execute(ctx, ReferenceExecutionRequest{
		Binding: environment.Binding, RootDir: environment.RootDir, Policy: environment.Policy,
		Request: append(json.RawMessage(nil), payload.Request...), Resume: payload.Resume,
		Checkpoint: cloneCheckpoint(environment.Checkpoint),
	})
	if err != nil {
		environment.State = PlacementStateFailed
		return worker.rejected(request, "execution_failed", "reference executor failed"), environment
	}
	if err := validateReferenceExecutionResult(result, environment.Policy, []string{payload.TaskToken}); err != nil {
		environment.State = PlacementStateFailed
		return worker.rejected(request, "result_rejected", "executor result exceeded policy or failed integrity checks"), environment
	}
	if err := worker.saveResultLocked(environment, result); err != nil {
		environment.State = PlacementStateFailed
		return worker.rejected(request, "result_rejected", "executor result could not be secured"), environment
	}
	worker.results[environment.PlacementID] = cloneReferenceExecutionResult(result)
	environment.ResultDigest = digestBytes(result.Payload)
	if result.Checkpoint != nil {
		environment.Checkpoint = &Checkpoint{
			ID: result.Checkpoint.ID, Digest: result.Checkpoint.Digest,
			URI: strings.TrimSpace(result.Checkpoint.URI), CreatedAt: worker.now().UTC(),
		}
		environment.State = PlacementStateCheckpointed
	} else {
		environment.State = PlacementStateCollecting
	}
	response := worker.accepted(request, result.Payload, result.Artifacts)
	if result.Checkpoint != nil {
		checkpoint := *result.Checkpoint
		checkpoint.Metadata = append(json.RawMessage(nil), result.Checkpoint.Metadata...)
		response.Checkpoint = &checkpoint
	}
	return response, environment
}

func (worker *ReferenceWorker) checkpointLocked(request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	if environment.State != PlacementStateCheckpointed || environment.Checkpoint == nil {
		return worker.rejected(request, "invalid_transition", "placement has no checkpoint to acknowledge"), environment
	}
	var payload CheckpointPayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil ||
		payload.ID != environment.Checkpoint.ID || payload.Digest != environment.Checkpoint.Digest {
		return worker.rejected(request, "checkpoint_mismatch", "checkpoint acknowledgment does not match execution"), environment
	}
	responsePayload, _ := json.Marshal(map[string]any{"checkpoint_id": payload.ID, "digest": payload.Digest})
	return worker.accepted(request, responsePayload, nil), environment
}

func (worker *ReferenceWorker) collectLocked(request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	if environment.State != PlacementStateCollecting && environment.State != PlacementStateCheckpointed {
		return worker.rejected(request, "invalid_transition", "placement has no execution result to collect"), environment
	}
	var payload CollectPayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
		return worker.rejected(request, "invalid_collect", "artifact collection request is invalid"), environment
	}
	if _, err := worker.verifier.Verify(payload.TaskToken, environment.Binding, TaskScopeCollect); err != nil {
		return worker.rejected(request, "authorization_denied", "task authorization was denied"), environment
	}
	result, ok := worker.results[environment.PlacementID]
	if !ok {
		var err error
		result, err = worker.loadResultLocked(environment)
		if err != nil {
			return worker.rejected(request, "result_unavailable", "execution result is unavailable"), environment
		}
		worker.results[environment.PlacementID] = cloneReferenceExecutionResult(result)
	}
	if payload.Complete {
		if payload.Succeeded {
			environment.State = PlacementStateCompleted
		} else {
			environment.State = PlacementStateFailed
		}
	}
	response := worker.accepted(request, result.Payload, result.Artifacts)
	response.Checkpoint = cloneCheckpointPayload(result.Checkpoint)
	return response, environment
}

func (worker *ReferenceWorker) destroyLocked(request WireRequest, environment ReferenceEnvironment) (WireResponse, ReferenceEnvironment) {
	var payload DestroyPayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil {
		return worker.rejected(request, "invalid_destroy", "destroy request is invalid"), environment
	}
	if _, err := worker.verifier.Verify(payload.TaskToken, environment.Binding, TaskScopeDestroy); err != nil {
		return worker.rejected(request, "authorization_denied", "task authorization was denied"), environment
	}
	if environment.State != PlacementStateCompleted && environment.State != PlacementStateFailed && environment.State != PlacementStateLost && environment.State != PlacementStateReclaiming {
		return worker.rejected(request, "invalid_transition", "placement is not destroyable"), environment
	}
	if !sameOrWithinWorkspace(worker.placements, environment.RootDir) || filepath.Clean(environment.RootDir) == filepath.Clean(worker.placements) {
		return worker.rejected(request, "destroy_denied", "placement path is unsafe to destroy"), environment
	}
	if err := os.RemoveAll(environment.RootDir); err != nil {
		return worker.rejected(request, "destroy_failed", "placement could not be destroyed"), environment
	}
	delete(worker.results, environment.PlacementID)
	environment.State = PlacementStateDestroyed
	environment.LeaseExpiresAt = nil
	return worker.accepted(request, json.RawMessage(`{"destroyed":true}`), nil), environment
}

func (worker *ReferenceWorker) Snapshot() ReferenceWorkerSnapshot {
	if worker == nil {
		return ReferenceWorkerSnapshot{Environments: map[string]ReferenceEnvironment{}}
	}
	worker.mu.Lock()
	defer worker.mu.Unlock()
	environments := make(map[string]ReferenceEnvironment, len(worker.environments))
	for id, environment := range worker.environments {
		environment.Policy.Egress.AllowHosts = append([]string(nil), environment.Policy.Egress.AllowHosts...)
		environment.Checkpoint = cloneCheckpoint(environment.Checkpoint)
		if environment.LeaseExpiresAt != nil {
			expiresAt := *environment.LeaseExpiresAt
			environment.LeaseExpiresAt = &expiresAt
		}
		environments[id] = environment
	}
	return ReferenceWorkerSnapshot{WorkerID: worker.workerID, Capabilities: worker.capabilities, Environments: environments}
}

func (worker *ReferenceWorker) loadState() error {
	raw, err := os.ReadFile(worker.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workerprotocol: read reference worker state: %w", err)
	}
	var state referenceWorkerState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("workerprotocol: decode reference worker state: %w", err)
	}
	if state.SchemaVersion != referenceWorkerSchemaVersion || state.WorkerID != worker.workerID {
		return fmt.Errorf("%w: incompatible reference worker state", ErrVersionUnsupported)
	}
	if state.Environments == nil {
		state.Environments = make(map[string]ReferenceEnvironment)
	}
	if state.AppliedIDs == nil {
		state.AppliedIDs = make(map[string]referenceApplied)
	}
	if state.AppliedKeys == nil {
		state.AppliedKeys = make(map[string]referenceApplied)
	}
	for placementID, environment := range state.Environments {
		if err := worker.validatePersistedEnvironment(placementID, environment); err != nil {
			return err
		}
	}
	for key, receipt := range state.AppliedIDs {
		if !validProtocolIdentifier(key) || !validReferenceReceipt(receipt) {
			return fmt.Errorf("%w: invalid persisted worker receipt", ErrWireContract)
		}
	}
	for key, receipt := range state.AppliedKeys {
		if !validProtocolIdentifier(key) || !validReferenceReceipt(receipt) {
			return fmt.Errorf("%w: invalid persisted worker idempotency receipt", ErrWireContract)
		}
	}
	worker.environments = state.Environments
	worker.appliedIDs = state.AppliedIDs
	worker.appliedKeys = state.AppliedKeys
	return nil
}

func (worker *ReferenceWorker) saveStateLocked() error {
	state := referenceWorkerState{
		SchemaVersion: referenceWorkerSchemaVersion, WorkerID: worker.workerID,
		Environments: worker.environments, AppliedIDs: worker.appliedIDs, AppliedKeys: worker.appliedKeys,
		UpdatedAt: worker.now().UTC(),
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("workerprotocol: encode reference worker state: %w", err)
	}
	if err := atomicwrite.Write(worker.statePath, append(raw, '\n')); err != nil {
		return fmt.Errorf("workerprotocol: save reference worker state: %w", err)
	}
	if err := os.Chmod(worker.statePath, 0o600); err != nil {
		return fmt.Errorf("workerprotocol: secure reference worker state: %w", err)
	}
	return nil
}

func (worker *ReferenceWorker) validatePersistedEnvironment(placementID string, environment ReferenceEnvironment) error {
	expectedRoot := filepath.Join(worker.placements, placementID)
	if !validProtocolIdentifier(placementID) || environment.PlacementID != placementID ||
		environment.Binding.PlacementID != placementID || environment.Binding.WorkerID != worker.workerID ||
		filepath.Clean(environment.RootDir) != filepath.Clean(expectedRoot) || environment.LastSequence < 1 ||
		!validPlacementState(environment.State) {
		return fmt.Errorf("%w: invalid persisted reference environment %q", ErrWireContract, placementID)
	}
	if err := validateTaskTokenBinding(environment.Binding); err != nil {
		return err
	}
	if err := environment.Policy.Validate(); err != nil {
		return err
	}
	return nil
}

func validReferenceReceipt(receipt referenceApplied) bool {
	return strings.TrimSpace(receipt.Fingerprint) != "" && validMessageType(receipt.Type) &&
		(receipt.PlacementID == "" || validProtocolIdentifier(receipt.PlacementID))
}

func validPlacementState(state PlacementState) bool {
	switch state {
	case PlacementStatePending, PlacementStateProvisioning, PlacementStateSyncing, PlacementStateReady,
		PlacementStateExecuting, PlacementStateCheckpointed, PlacementStateCollecting,
		PlacementStateCompleted, PlacementStateFailed, PlacementStateLost, PlacementStateReclaiming,
		PlacementStateRehydrating, PlacementStateDestroyed:
		return true
	default:
		return false
	}
}

func (worker *ReferenceWorker) saveResultLocked(environment ReferenceEnvironment, result ReferenceExecutionResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("workerprotocol: encode reference execution result: %w", err)
	}
	path := worker.resultPath(environment)
	if !sameOrWithinWorkspace(environment.RootDir, path) {
		return ErrUnsafeWorkspace
	}
	if err := atomicwrite.Write(path, append(raw, '\n')); err != nil {
		return fmt.Errorf("workerprotocol: save reference execution result: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("workerprotocol: secure reference execution result: %w", err)
	}
	return nil
}

func (worker *ReferenceWorker) loadResultLocked(environment ReferenceEnvironment) (ReferenceExecutionResult, error) {
	path := worker.resultPath(environment)
	if !sameOrWithinWorkspace(environment.RootDir, path) {
		return ReferenceExecutionResult{}, ErrUnsafeWorkspace
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ReferenceExecutionResult{}, fmt.Errorf("workerprotocol: read reference execution result: %w", err)
	}
	var result ReferenceExecutionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ReferenceExecutionResult{}, fmt.Errorf("workerprotocol: decode reference execution result: %w", err)
	}
	if err := validateReferenceExecutionResult(result, environment.Policy, nil); err != nil {
		return ReferenceExecutionResult{}, err
	}
	if environment.ResultDigest != "" && digestBytes(result.Payload) != environment.ResultDigest {
		return ReferenceExecutionResult{}, ErrManifestMismatch
	}
	return result, nil
}

func (worker *ReferenceWorker) resultPath(environment ReferenceEnvironment) string {
	return filepath.Join(environment.RootDir, ".tars-result.json")
}

func (worker *ReferenceWorker) replayLocked(request WireRequest, applied referenceApplied) (WireResponse, error) {
	if applied.Response.RequestID != "" {
		response := cloneWireResponse(applied.Response)
		response.RequestID = request.RequestID
		return response, nil
	}
	switch applied.Type {
	case MessageRegister:
		payload, _ := json.Marshal(RegisterPayload{
			Transport: "in-process", Endpoint: "local://" + worker.workerID, Capabilities: worker.capabilities,
			VerificationKeyID: worker.VerificationKeyID(),
		})
		return worker.accepted(request, payload, nil), nil
	case MessageExecute, MessageCollect:
		environment, ok := worker.environments[applied.PlacementID]
		if !ok || environment.State == PlacementStateDestroyed {
			return WireResponse{}, fmt.Errorf("workerprotocol: replay result is unavailable")
		}
		result, err := worker.loadResultLocked(environment)
		if err != nil {
			return WireResponse{}, err
		}
		response := worker.accepted(request, result.Payload, result.Artifacts)
		if applied.Type == MessageExecute {
			response.Checkpoint = cloneCheckpointPayload(result.Checkpoint)
		}
		return response, nil
	default:
		return worker.accepted(request, json.RawMessage(`{"replayed":true}`), nil), nil
	}
}

func (worker *ReferenceWorker) accepted(request WireRequest, payload json.RawMessage, artifacts []WireArtifact) WireResponse {
	return WireResponse{
		ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true,
		Payload: append(json.RawMessage(nil), payload...), Artifacts: cloneWireArtifacts(artifacts),
	}
}

func (worker *ReferenceWorker) rejected(request WireRequest, code, message string) WireResponse {
	return WireResponse{
		ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID,
		Accepted: false, ErrorCode: code, Error: message,
	}
}

func validateReferenceExecutionResult(result ReferenceExecutionResult, policy ExecutionPolicy, forbiddenValues []string) error {
	if len(result.Payload) > 0 && !json.Valid(result.Payload) {
		return ErrWireContract
	}
	var totalBytes int64 = int64(len(result.Payload))
	seen := make(map[string]struct{}, len(result.Artifacts))
	limits := DefaultArtifactQuarantineLimits()
	for _, artifact := range result.Artifacts {
		if reason := validateWireArtifact(artifact, limits, &totalBytes, seen); reason != "" {
			return ErrManifestMismatch
		}
		if containsForbiddenResultValue(artifact.Data, forbiddenValues) {
			return ErrTaskTokenInvalid
		}
	}
	if len(result.Payload) > 0 {
		if containsForbiddenResultValue(result.Payload, forbiddenValues) {
			return ErrTaskTokenInvalid
		}
	}
	if totalBytes > policy.Limits.MaxOutputBytes {
		return ErrTransportLimit
	}
	if result.Checkpoint != nil && (!validProtocolIdentifier(result.Checkpoint.ID) || strings.TrimSpace(result.Checkpoint.Digest) == "") {
		return ErrManifestMismatch
	}
	return nil
}

func containsForbiddenResultValue(raw []byte, forbiddenValues []string) bool {
	for _, value := range forbiddenValues {
		if value != "" && strings.Contains(string(raw), value) {
			return true
		}
	}
	return false
}

func wireRequestFingerprint(request WireRequest) string {
	fingerprint := envelopeFingerprint(request.Envelope)
	if request.Workspace != nil {
		fingerprint += ":" + request.Workspace.Manifest.Digest
	}
	return fingerprint
}

func cloneWireResponse(response WireResponse) WireResponse {
	response.Payload = append(json.RawMessage(nil), response.Payload...)
	response.Artifacts = cloneWireArtifacts(response.Artifacts)
	if response.Checkpoint != nil {
		checkpoint := *response.Checkpoint
		checkpoint.Metadata = append(json.RawMessage(nil), response.Checkpoint.Metadata...)
		response.Checkpoint = &checkpoint
	}
	return response
}

func cloneWireArtifacts(artifacts []WireArtifact) []WireArtifact {
	cloned := make([]WireArtifact, len(artifacts))
	for index, artifact := range artifacts {
		artifact.Data = append([]byte(nil), artifact.Data...)
		cloned[index] = artifact
	}
	return cloned
}

func cloneReferenceExecutionResult(result ReferenceExecutionResult) ReferenceExecutionResult {
	result.Payload = append(json.RawMessage(nil), result.Payload...)
	result.Artifacts = cloneWireArtifacts(result.Artifacts)
	if result.Checkpoint != nil {
		checkpoint := *result.Checkpoint
		checkpoint.Metadata = append(json.RawMessage(nil), checkpoint.Metadata...)
		result.Checkpoint = &checkpoint
	}
	return result
}

func cloneCheckpoint(checkpoint *Checkpoint) *Checkpoint {
	if checkpoint == nil {
		return nil
	}
	copy := *checkpoint
	return &copy
}

var _ WireHandler = (*ReferenceWorker)(nil)
