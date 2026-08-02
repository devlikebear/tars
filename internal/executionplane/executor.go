package executionplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
)

const defaultCredentialTTL = 15 * time.Minute

type Options struct {
	Adapter           string
	SourceDir         string
	Provider          EnvironmentProvider
	Worker            WorkerClient
	CredentialBroker  CredentialBroker
	ArtifactCollector ArtifactCollector
	ArtifactSink      ArtifactSink
	StateStore        StateStore
	EventSink         EventSink
	MaxCredentialTTL  time.Duration
	Now               func() time.Time
}

type LifecycleExecutor struct {
	adapter           string
	sourceDir         string
	provider          EnvironmentProvider
	worker            WorkerClient
	credentialBroker  CredentialBroker
	artifactCollector ArtifactCollector
	artifactSink      ArtifactSink
	stateStore        StateStore
	eventSink         EventSink
	maxCredentialTTL  time.Duration
	now               func() time.Time
}

func NewLifecycleExecutor(opts Options) (*LifecycleExecutor, error) {
	adapter := strings.TrimSpace(opts.Adapter)
	if adapter == "" {
		return nil, fmt.Errorf("executionplane: adapter is required")
	}
	if opts.Provider == nil || strings.TrimSpace(opts.Provider.Name()) == "" {
		return nil, fmt.Errorf("executionplane: environment provider is required")
	}
	if opts.Worker == nil || strings.TrimSpace(opts.Worker.Name()) == "" {
		return nil, fmt.Errorf("executionplane: worker client is required")
	}
	if opts.StateStore == nil {
		return nil, fmt.Errorf("executionplane: durable state store is required")
	}
	if strings.TrimSpace(opts.SourceDir) == "" {
		return nil, fmt.Errorf("executionplane: source directory is required")
	}
	if opts.MaxCredentialTTL <= 0 {
		opts.MaxCredentialTTL = defaultCredentialTTL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &LifecycleExecutor{
		adapter: adapter, sourceDir: strings.TrimSpace(opts.SourceDir), provider: opts.Provider,
		worker: opts.Worker, credentialBroker: opts.CredentialBroker,
		artifactCollector: opts.ArtifactCollector, artifactSink: opts.ArtifactSink,
		stateStore: opts.StateStore, eventSink: opts.EventSink,
		maxCredentialTTL: opts.MaxCredentialTTL, now: opts.Now,
	}, nil
}

func (executor *LifecycleExecutor) Adapter() string {
	if executor == nil {
		return ""
	}
	return executor.adapter
}

func (executor *LifecycleExecutor) Capabilities() ExecutorCapabilities {
	if executor == nil || executor.worker == nil || executor.provider == nil {
		return ExecutorCapabilities{}
	}
	capabilities := executor.worker.Capabilities()
	capabilities.Resume = capabilities.Resume && executor.provider.Capabilities().Recoverable
	capabilities.Artifacts = capabilities.Artifacts || executor.artifactCollector != nil
	return capabilities
}

func (executor *LifecycleExecutor) Descriptor() AdapterDescriptor {
	if executor == nil {
		return AdapterDescriptor{}
	}
	descriptor := AdapterDescriptor{Adapter: executor.adapter, Executor: executor.Capabilities()}
	if executor.provider != nil {
		descriptor.Provider = executor.provider.Name()
		descriptor.Environment = executor.provider.Capabilities()
	}
	if executor.worker != nil {
		descriptor.Worker = executor.worker.Name()
	}
	return descriptor
}

func (executor *LifecycleExecutor) Execute(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	if executor == nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("executionplane: executor is not configured")
	}
	environment, err := executor.provider.Provision(ctx, ProvisionRequest{Execution: execution, SourceDir: executor.sourceDir})
	if err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("executionplane: provision %s environment: %w", executor.provider.Name(), err)
	}
	if err := validateEnvironment(environment); err != nil {
		_ = executor.provider.Destroy(context.Background(), environment)
		return workscheduler.ExecutionResult{}, err
	}
	state := LifecycleState{
		SchemaVersion: lifecycleSchemaVersion, AttemptID: execution.Claim.Attempt.ID,
		Phase: EventEnvironmentProvisioned, Environment: environment, UpdatedAt: executor.now().UTC(),
	}
	if err := executor.saveAndRecord(ctx, execution, &state, LifecycleEvent{Phase: EventEnvironmentProvisioned}); err != nil {
		_ = executor.provider.Destroy(context.Background(), environment)
		return workscheduler.ExecutionResult{}, err
	}

	grant, err := executor.issueCredentials(ctx, execution, environment)
	if err != nil {
		cleanupErr := executor.cleanup(context.Background(), execution, &state, CredentialGrant{})
		return workscheduler.ExecutionResult{}, errors.Join(err, cleanupErr)
	}
	if grant.ID != "" {
		state.CredentialID = grant.ID
		state.Phase = EventCredentialsIssued
		if err := executor.saveAndRecord(ctx, execution, &state, LifecycleEvent{Phase: EventCredentialsIssued, CredentialID: grant.ID}); err != nil {
			cleanupErr := executor.cleanup(context.Background(), execution, &state, grant)
			return workscheduler.ExecutionResult{}, errors.Join(err, cleanupErr)
		}
	}

	state.Phase = EventWorkerStarted
	if err := executor.saveAndRecord(ctx, execution, &state, LifecycleEvent{Phase: EventWorkerStarted}); err != nil {
		cleanupErr := executor.cleanup(context.Background(), execution, &state, grant)
		return workscheduler.ExecutionResult{}, errors.Join(err, cleanupErr)
	}
	workerResult, workerErr := executor.worker.Execute(ctx, WorkerRequest{Execution: execution, Environment: environment, Credentials: grant})
	result, finalizeErr := executor.finalize(ctx, execution, &state, grant, workerResult)
	return result, errors.Join(workerErr, finalizeErr)
}

func (executor *LifecycleExecutor) Recover(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, bool, error) {
	if executor == nil {
		return workscheduler.ExecutionResult{}, false, fmt.Errorf("executionplane: executor is not configured")
	}
	state, found, err := executor.stateStore.Load(ctx, execution.Claim.Attempt.ID)
	if err != nil || !found {
		return workscheduler.ExecutionResult{}, false, err
	}
	if !executor.Capabilities().Resume {
		return workscheduler.ExecutionResult{}, true, fmt.Errorf("%w: executor %q cannot resume", ErrUnsupported, executor.adapter)
	}
	environment, err := executor.provider.Recover(ctx, state.Environment)
	if err != nil {
		return workscheduler.ExecutionResult{}, true, fmt.Errorf("executionplane: recover environment: %w", err)
	}
	state.Environment = environment
	state.Phase = EventRecoveryStarted
	if err := executor.saveAndRecord(ctx, execution, &state, LifecycleEvent{Phase: EventRecoveryStarted}); err != nil {
		return workscheduler.ExecutionResult{}, true, err
	}
	grant, err := executor.issueCredentials(ctx, execution, environment)
	if err != nil {
		return workscheduler.ExecutionResult{}, true, err
	}
	workerResult, recovered, workerErr := executor.worker.Recover(ctx, WorkerRequest{
		Execution: execution, Environment: environment, Credentials: grant,
	}, state.Checkpoint)
	if !recovered {
		cleanupErr := executor.cleanup(context.Background(), execution, &state, grant)
		return workscheduler.ExecutionResult{}, false, errors.Join(workerErr, cleanupErr)
	}
	result, finalizeErr := executor.finalize(ctx, execution, &state, grant, workerResult)
	return result, true, errors.Join(workerErr, finalizeErr)
}

func (executor *LifecycleExecutor) Cancel(ctx context.Context, execution workscheduler.Execution) error {
	if executor == nil || !executor.Capabilities().Cancellation {
		return fmt.Errorf("%w: executor cancellation", ErrUnsupported)
	}
	state, found, err := executor.stateStore.Load(ctx, execution.Claim.Attempt.ID)
	if err != nil || !found {
		return err
	}
	environment, err := executor.provider.Recover(ctx, state.Environment)
	if err != nil {
		return err
	}
	request := WorkerRequest{Execution: execution, Environment: environment}
	if err := executor.worker.Cancel(ctx, request); err != nil {
		return err
	}
	if err := executor.record(ctx, execution, state, LifecycleEvent{Phase: EventWorkerCancelled}); err != nil {
		return err
	}
	return executor.cleanup(context.Background(), execution, &state, CredentialGrant{ID: state.CredentialID})
}

func (executor *LifecycleExecutor) issueCredentials(ctx context.Context, execution workscheduler.Execution, environment Environment) (CredentialGrant, error) {
	if executor.credentialBroker == nil {
		return CredentialGrant{}, nil
	}
	grant, err := executor.credentialBroker.Issue(ctx, CredentialRequest{
		Execution: execution, Environment: environment, Worker: executor.worker.Name(),
	})
	if err != nil {
		return CredentialGrant{}, fmt.Errorf("executionplane: issue credentials: %w", err)
	}
	if err := executor.validateCredentialGrant(grant); err != nil {
		if strings.TrimSpace(grant.ID) != "" {
			_ = executor.credentialBroker.Revoke(context.Background(), grant)
		}
		return CredentialGrant{}, err
	}
	return grant, nil
}

func (executor *LifecycleExecutor) validateCredentialGrant(grant CredentialGrant) error {
	if len(grant.Values) == 0 {
		if strings.TrimSpace(grant.ID) == "" {
			return nil
		}
		return fmt.Errorf("%w: grant %q has no in-memory values", ErrCredentialScope, grant.ID)
	}
	if strings.TrimSpace(grant.ID) == "" || grant.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: grant id and expiry are required", ErrCredentialScope)
	}
	now := executor.now().UTC()
	if !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(executor.maxCredentialTTL)) {
		return fmt.Errorf("%w: grant %q expiry exceeds %s", ErrCredentialScope, grant.ID, executor.maxCredentialTTL)
	}
	for key, value := range grant.Values {
		if strings.TrimSpace(key) == "" || value == "" {
			return fmt.Errorf("%w: grant %q contains an empty credential", ErrCredentialScope, grant.ID)
		}
	}
	return nil
}

func (executor *LifecycleExecutor) finalize(
	ctx context.Context,
	execution workscheduler.Execution,
	state *LifecycleState,
	grant CredentialGrant,
	workerResult WorkerResult,
) (workscheduler.ExecutionResult, error) {
	var operationErr error
	if workerResult.Checkpoint != nil {
		checkpoint := *workerResult.Checkpoint
		if checkpoint.CreatedAt.IsZero() {
			checkpoint.CreatedAt = executor.now().UTC()
		}
		state.Checkpoint = &checkpoint
		state.Phase = EventCheckpointRecorded
		operationErr = errors.Join(operationErr, executor.saveAndRecord(ctx, execution, state, LifecycleEvent{
			Phase: EventCheckpointRecorded, CheckpointID: checkpoint.ID,
		}))
	}
	snapshot, err := executor.provider.Sync(ctx, state.Environment)
	if err != nil {
		operationErr = errors.Join(operationErr, fmt.Errorf("executionplane: sync environment: %w", err))
	} else {
		if snapshot.CreatedAt.IsZero() {
			snapshot.CreatedAt = executor.now().UTC()
		}
		state.Snapshot = snapshot
		state.Phase = EventEnvironmentSynced
		operationErr = errors.Join(operationErr, executor.saveAndRecord(ctx, execution, state, LifecycleEvent{
			Phase: EventEnvironmentSynced, Snapshot: snapshot,
		}))
	}
	if executor.artifactCollector != nil {
		artifacts, collectErr := executor.artifactCollector.Collect(ctx, CollectRequest{
			Execution: execution, Environment: state.Environment, Snapshot: state.Snapshot,
			Worker: workerResult, RedactValues: credentialValues(grant),
		})
		if collectErr != nil {
			operationErr = errors.Join(operationErr, fmt.Errorf("executionplane: collect artifacts: %w", collectErr))
		} else {
			for _, artifact := range artifacts {
				if executor.artifactSink != nil {
					operationErr = errors.Join(operationErr, executor.artifactSink.StoreArtifact(ctx, execution, artifact))
				}
			}
			state.Phase = EventArtifactsCollected
			operationErr = errors.Join(operationErr, executor.saveAndRecord(ctx, execution, state, LifecycleEvent{
				Phase: EventArtifactsCollected, ArtifactCount: len(artifacts),
			}))
		}
	}
	operationErr = errors.Join(operationErr, executor.cleanup(context.Background(), execution, state, grant))
	return workerResult.ExecutionResult, operationErr
}

func credentialValues(grant CredentialGrant) []string {
	values := make([]string, 0, len(grant.Values))
	for _, value := range grant.Values {
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (executor *LifecycleExecutor) cleanup(ctx context.Context, execution workscheduler.Execution, state *LifecycleState, grant CredentialGrant) error {
	var cleanupErr error
	if executor.credentialBroker != nil && strings.TrimSpace(grant.ID) != "" {
		if err := executor.credentialBroker.Revoke(ctx, grant); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("executionplane: revoke credential: %w", err))
		} else {
			state.CredentialID = ""
			state.Phase = EventCredentialsRevoked
			cleanupErr = errors.Join(cleanupErr, executor.saveAndRecord(ctx, execution, state, LifecycleEvent{Phase: EventCredentialsRevoked, CredentialID: grant.ID}))
		}
	}
	if err := executor.provider.Destroy(ctx, state.Environment); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("executionplane: destroy environment: %w", err))
		_ = executor.stateStore.Save(ctx, *state)
		return cleanupErr
	}
	state.Phase = EventEnvironmentDestroyed
	cleanupErr = errors.Join(cleanupErr, executor.record(ctx, execution, *state, LifecycleEvent{Phase: EventEnvironmentDestroyed}))
	if cleanupErr == nil {
		cleanupErr = executor.stateStore.Delete(ctx, state.AttemptID)
	}
	return cleanupErr
}

func (executor *LifecycleExecutor) saveAndRecord(ctx context.Context, execution workscheduler.Execution, state *LifecycleState, event LifecycleEvent) error {
	state.UpdatedAt = executor.now().UTC()
	if err := executor.stateStore.Save(ctx, *state); err != nil {
		return fmt.Errorf("executionplane: save lifecycle state: %w", err)
	}
	return executor.record(ctx, execution, *state, event)
}

func (executor *LifecycleExecutor) record(ctx context.Context, execution workscheduler.Execution, state LifecycleState, event LifecycleEvent) error {
	if executor.eventSink == nil {
		return nil
	}
	event.Execution = execution
	event.Provider = executor.provider.Name()
	event.Worker = executor.worker.Name()
	event.EnvironmentID = state.Environment.ID
	event.OccurredAt = executor.now().UTC()
	if err := executor.eventSink.Record(ctx, event); err != nil {
		return fmt.Errorf("executionplane: record %s event: %w", event.Phase, err)
	}
	return nil
}

func validateEnvironment(environment Environment) error {
	if environment.SchemaVersion != lifecycleSchemaVersion || strings.TrimSpace(environment.ID) == "" || strings.TrimSpace(environment.Kind) == "" || strings.TrimSpace(environment.RootDir) == "" {
		return fmt.Errorf("executionplane: provider returned an invalid environment")
	}
	return nil
}

var _ Executor = (*LifecycleExecutor)(nil)
