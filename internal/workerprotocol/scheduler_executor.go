package workerprotocol

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

const remoteSchedulerRequestSchemaVersion = 1

type SchedulerRemoteCoordinator interface {
	Run(context.Context, RemoteRunInput) (RemoteRunResult, error)
	RecoverPrepared(context.Context, RemoteRunInput) (RemoteRunResult, error)
	FinalizeRecorded(context.Context, RemoteRunInput, RemoteRunResult) error
}

type SchedulerExecutorOptions struct {
	Adapter      string
	SourceDir    string
	SyncMode     SyncMode
	GitPath      string
	Policy       ExecutionPolicy
	BundleLimits WorkspaceBundleLimits
	Coordinator  SchedulerRemoteCoordinator
	Store        RemoteRunStore
}

type SchedulerExecutor struct {
	adapter      string
	sourceDir    string
	syncMode     SyncMode
	gitPath      string
	policy       ExecutionPolicy
	bundleLimits WorkspaceBundleLimits
	coordinator  SchedulerRemoteCoordinator
	store        RemoteRunStore
}

func NewSchedulerExecutor(opts SchedulerExecutorOptions) (*SchedulerExecutor, error) {
	adapter := strings.TrimSpace(opts.Adapter)
	sourceDir := strings.TrimSpace(opts.SourceDir)
	if adapter == "" || sourceDir == "" || opts.Coordinator == nil || opts.Store == nil ||
		(opts.SyncMode != SyncModeDirectory && opts.SyncMode != SyncModeGit) {
		return nil, fmt.Errorf("workerprotocol: remote scheduler executor is not configured")
	}
	absolute, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve remote scheduler source: %w", err)
	}
	if err := opts.Policy.Validate(); err != nil {
		return nil, err
	}
	limits := opts.BundleLimits
	if limits == (WorkspaceBundleLimits{}) {
		limits = DefaultWorkspaceBundleLimits()
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if opts.SyncMode == SyncModeGit && strings.TrimSpace(opts.GitPath) == "" {
		return nil, fmt.Errorf("workerprotocol: Git sync requires an explicit git executable")
	}
	return &SchedulerExecutor{
		adapter: adapter, sourceDir: absolute, syncMode: opts.SyncMode,
		gitPath: strings.TrimSpace(opts.GitPath), policy: opts.Policy, bundleLimits: limits,
		coordinator: opts.Coordinator, store: opts.Store,
	}, nil
}

func (executor *SchedulerExecutor) Adapter() string {
	if executor == nil {
		return ""
	}
	return executor.adapter
}

func (executor *SchedulerExecutor) Execute(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	if executor == nil || executor.coordinator == nil || executor.store == nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("workerprotocol: remote scheduler executor is not configured")
	}
	bundle, err := BuildWorkspaceBundle(ctx, WorkspaceBundleOptions{
		RootDir: executor.sourceDir, Mode: executor.syncMode, GitPath: executor.gitPath, Limits: executor.bundleLimits,
	})
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	input, err := executor.inputForExecution(execution, bundle)
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	if err := executor.store.Prepare(ctx, input); err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	result, err := executor.coordinator.Run(ctx, input)
	if err != nil {
		return executor.recordedResultAfterCoordinatorError(ctx, input, err)
	}
	if err := executor.store.RecordResult(ctx, input, result); err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	return schedulerExecutionResult(result)
}

func (executor *SchedulerExecutor) Recover(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, bool, error) {
	if executor == nil || executor.coordinator == nil || executor.store == nil {
		return workscheduler.ExecutionResult{}, false, fmt.Errorf("workerprotocol: remote scheduler executor is not configured")
	}
	state, found, err := executor.store.Load(ctx, execution.Claim.Attempt.ID)
	if err != nil || !found {
		return workscheduler.ExecutionResult{}, found, err
	}
	if !remoteRunStateMatchesExecution(state, execution) {
		return workscheduler.ExecutionResult{}, true, fmt.Errorf("%w: remote run state does not match scheduler claim", ErrConflict)
	}
	if state.Result != nil {
		if err := executor.coordinator.FinalizeRecorded(ctx, state.Input, *state.Result); err != nil {
			return workscheduler.ExecutionResult{}, true, err
		}
		result, err := schedulerExecutionResult(*state.Result)
		return result, true, err
	}
	remoteResult, err := executor.coordinator.RecoverPrepared(ctx, cloneRemoteRunInput(state.Input))
	if err != nil {
		result, recordedErr := executor.recordedResultAfterCoordinatorError(ctx, state.Input, err)
		return result, true, recordedErr
	}
	if err := executor.store.RecordResult(ctx, state.Input, remoteResult); err != nil {
		return workscheduler.ExecutionResult{}, true, err
	}
	result, err := schedulerExecutionResult(remoteResult)
	return result, true, err
}

func (executor *SchedulerExecutor) Finalize(ctx context.Context, execution workscheduler.Execution, _ workscheduler.ExecutionResult) error {
	if executor == nil || executor.store == nil || executor.coordinator == nil {
		return fmt.Errorf("workerprotocol: remote scheduler executor is not configured")
	}
	state, found, err := executor.store.Load(ctx, execution.Claim.Attempt.ID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !remoteRunStateMatchesExecution(state, execution) || state.Result == nil {
		return fmt.Errorf("%w: remote run is not ready for finalization", ErrConflict)
	}
	if err := executor.coordinator.FinalizeRecorded(ctx, state.Input, *state.Result); err != nil {
		return err
	}
	return executor.store.Delete(ctx, execution.Claim.Attempt.ID)
}

func (executor *SchedulerExecutor) recordedResultAfterCoordinatorError(
	ctx context.Context,
	input RemoteRunInput,
	coordinatorErr error,
) (workscheduler.ExecutionResult, error) {
	state, found, err := executor.store.Load(ctx, input.AttemptID)
	if err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("%w: read durable remote result after coordinator failure: %v", coordinatorErr, err)
	}
	if !found || state.Result == nil || !remoteRunInputEqual(state.Input, cloneRemoteRunInput(input)) {
		return workscheduler.ExecutionResult{}, coordinatorErr
	}
	return schedulerExecutionResult(*state.Result)
}

func (executor *SchedulerExecutor) inputForExecution(execution workscheduler.Execution, bundle WorkspaceBundle) (RemoteRunInput, error) {
	request, err := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		WorkTitle     string `json:"work_title"`
		Objective     string `json:"objective,omitempty"`
		StepTitle     string `json:"step_title"`
		Instructions  string `json:"instructions,omitempty"`
	}{
		SchemaVersion: remoteSchedulerRequestSchemaVersion, WorkTitle: execution.Work.Title,
		Objective: execution.Work.Objective, StepTitle: execution.Claim.Step.Title,
		Instructions: execution.Claim.Step.Description,
	})
	if err != nil {
		return RemoteRunInput{}, fmt.Errorf("workerprotocol: encode remote scheduler request: %w", err)
	}
	attemptID := strings.TrimSpace(execution.Claim.Attempt.ID)
	if !safeRemoteRunID.MatchString(attemptID) || !validProtocolIdentifier("placement-"+attemptID) ||
		!validProtocolIdentifier("environment-"+attemptID) {
		return RemoteRunInput{}, fmt.Errorf("workerprotocol: scheduler attempt id cannot form a remote placement")
	}
	input := RemoteRunInput{
		PlacementID: "placement-" + attemptID, EnvironmentID: "environment-" + attemptID,
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID,
		StepID: execution.Claim.Step.ID, AttemptID: attemptID, Policy: executor.policy,
		Workspace: cloneWorkspaceBundle(bundle), Request: request,
	}
	if err := validateRemoteRunInput(input); err != nil {
		return RemoteRunInput{}, err
	}
	return input, nil
}

func schedulerExecutionResult(result RemoteRunResult) (workscheduler.ExecutionResult, error) {
	if err := validatePersistedRemoteResult(result); err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	payload := result.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	output, err := json.Marshal(struct {
		Result            json.RawMessage    `json:"result"`
		Artifacts         []ReleasedArtifact `json:"artifacts,omitempty"`
		RejectedArtifacts []RejectedArtifact `json:"rejected_artifacts,omitempty"`
		Checkpoint        *CheckpointPayload `json:"checkpoint,omitempty"`
	}{
		Result: payload, Artifacts: result.Artifacts,
		RejectedArtifacts: result.RejectedArtifacts, Checkpoint: result.Checkpoint,
	})
	if err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("workerprotocol: encode scheduler remote result: %w", err)
	}
	errorText := ""
	if !result.Succeeded {
		errorText = "remote worker reported unsuccessful execution"
	}
	return workscheduler.ExecutionResult{
		Succeeded: result.Succeeded, OutputJSON: output, Error: errorText,
		Usage: workstore.StepAttemptUsage{Iterations: 1},
	}, nil
}

func remoteRunStateMatchesExecution(state RemoteRunState, execution workscheduler.Execution) bool {
	return state.AttemptID == execution.Claim.Attempt.ID && state.Input.AttemptID == execution.Claim.Attempt.ID &&
		state.Input.WorkspaceID == execution.Work.WorkspaceID && state.Input.WorkID == execution.Work.ID &&
		state.Input.StepID == execution.Claim.Step.ID
}

var _ workscheduler.Executor = (*SchedulerExecutor)(nil)
var _ workscheduler.RecoverableExecutor = (*SchedulerExecutor)(nil)
var _ workscheduler.FinalizableExecutor = (*SchedulerExecutor)(nil)
