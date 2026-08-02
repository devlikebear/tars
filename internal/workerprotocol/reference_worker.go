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
)

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
	fingerprint string
	response    WireResponse
}

type ReferenceWorker struct {
	mu           sync.Mutex
	workerID     string
	rootDir      string
	placements   string
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
	return &ReferenceWorker{
		workerID: workerID, rootDir: canonical, placements: placements, verifier: opts.TokenVerifier,
		executor: opts.Executor, capabilities: capabilities, now: opts.Now,
		environments: make(map[string]ReferenceEnvironment), results: make(map[string]ReferenceExecutionResult),
		appliedIDs: make(map[string]referenceApplied), appliedKeys: make(map[string]referenceApplied),
	}, nil
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
		if applied.fingerprint != fingerprint {
			return worker.rejected(request, "idempotency_conflict", "request id changed content"), nil
		}
		return cloneWireResponse(applied.response), nil
	}
	if applied, ok := worker.appliedKeys[request.Envelope.IdempotencyKey]; ok {
		if applied.fingerprint != fingerprint {
			return worker.rejected(request, "idempotency_conflict", "idempotency key changed content"), nil
		}
		return cloneWireResponse(applied.response), nil
	}

	response := worker.handleLocked(ctx, request)
	if response.Accepted {
		applied := referenceApplied{fingerprint: fingerprint, response: cloneWireResponse(response)}
		worker.appliedIDs[request.RequestID] = applied
		worker.appliedKeys[request.Envelope.IdempotencyKey] = applied
	}
	return response, nil
}

func (worker *ReferenceWorker) handleLocked(ctx context.Context, request WireRequest) WireResponse {
	envelope := request.Envelope
	if envelope.PlacementID == "" {
		switch envelope.Type {
		case MessageRegister:
			payload, _ := json.Marshal(RegisterPayload{
				Transport: "in-process", Endpoint: "local://" + worker.workerID, Capabilities: worker.capabilities,
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
	if !exists && (envelope.Type != MessageProvision || envelope.Sequence != 1) {
		return worker.rejected(request, "environment_not_found", "placement environment is unavailable")
	}

	var response WireResponse
	switch envelope.Type {
	case MessageProvision:
		response, environment = worker.provisionLocked(request, environment, exists)
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
	if environment.State != PlacementStateReady && environment.State != PlacementStateCheckpointed {
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
	if err := validateReferenceExecutionResult(result, environment.Policy); err != nil {
		environment.State = PlacementStateFailed
		return worker.rejected(request, "result_rejected", "executor result exceeded policy or failed integrity checks"), environment
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
		return worker.rejected(request, "result_unavailable", "execution result is unavailable"), environment
	}
	if payload.Complete {
		if payload.Succeeded {
			environment.State = PlacementStateCompleted
		} else {
			environment.State = PlacementStateFailed
		}
	}
	return worker.accepted(request, result.Payload, result.Artifacts), environment
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

func validateReferenceExecutionResult(result ReferenceExecutionResult, policy ExecutionPolicy) error {
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
	}
	if totalBytes > policy.Limits.MaxOutputBytes {
		return ErrTransportLimit
	}
	if result.Checkpoint != nil && (!validProtocolIdentifier(result.Checkpoint.ID) || strings.TrimSpace(result.Checkpoint.Digest) == "") {
		return ErrManifestMismatch
	}
	return nil
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
