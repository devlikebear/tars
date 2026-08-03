package executionplane

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestLifecycleExecutorRunsObservableContract(t *testing.T) {
	t.Parallel()

	var calls []string
	provider := &fakeEnvironmentProvider{
		name:         "local",
		capabilities: EnvironmentCapabilities{Recoverable: true, Snapshot: true, Cleanup: true},
		provision: func(_ context.Context, request ProvisionRequest) (Environment, error) {
			calls = append(calls, "provision")
			return Environment{SchemaVersion: 1, ID: "env-1", Kind: "local", RootDir: request.SourceDir}, nil
		},
		sync: func(context.Context, Environment) (EnvironmentSnapshot, error) {
			calls = append(calls, "sync")
			return EnvironmentSnapshot{Digest: "sha256:snapshot", URI: "file:///snapshot.json"}, nil
		},
		destroy: func(context.Context, Environment) error {
			calls = append(calls, "destroy")
			return nil
		},
	}
	worker := &fakeWorkerClient{
		name:         "native",
		capabilities: ExecutorCapabilities{Resume: true, Cancellation: true, Transcript: true, Cost: true},
		execute: func(_ context.Context, request WorkerRequest) (WorkerResult, error) {
			calls = append(calls, "execute")
			if request.Environment.ID != "env-1" || request.Credentials.ID != "grant-1" || request.Credentials.Values["TASK_TOKEN"] != "ephemeral" {
				t.Fatalf("worker request = %#v", request)
			}
			return WorkerResult{
				ExecutionResult: workscheduler.ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"ok":true}`)},
				Checkpoint:      &WorkerCheckpoint{ID: "checkpoint-1", ResumeToken: "resume-1"},
			}, nil
		},
	}
	broker := &fakeCredentialBroker{
		issue: func(context.Context, CredentialRequest) (CredentialGrant, error) {
			calls = append(calls, "issue")
			return CredentialGrant{ID: "grant-1", Values: map[string]string{"TASK_TOKEN": "ephemeral"}, ExpiresAt: time.Now().Add(time.Minute)}, nil
		},
		revoke: func(context.Context, CredentialGrant) error {
			calls = append(calls, "revoke")
			return nil
		},
	}
	collector := &fakeArtifactCollector{collect: func(_ context.Context, request CollectRequest) ([]CollectedArtifact, error) {
		calls = append(calls, "collect")
		if request.Snapshot.Digest != "sha256:snapshot" {
			t.Fatalf("collector snapshot = %#v", request.Snapshot)
		}
		return []CollectedArtifact{{Kind: "log", Name: "run.log", URI: "file:///run.log", Digest: "sha256:log"}}, nil
	}}
	states := newMemoryStateStore()
	events := &memoryEventSink{}
	executor, err := NewLifecycleExecutor(Options{
		Adapter: "native-local", SourceDir: "/workspace", Provider: provider, Worker: worker,
		CredentialBroker: broker, ArtifactCollector: collector, StateStore: states, EventSink: events,
		MaxCredentialTTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new lifecycle executor: %v", err)
	}

	execution := testExecution()
	result, err := executor.Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("execute lifecycle: %v", err)
	}
	if !result.Succeeded || string(result.OutputJSON) != `{"ok":true}` {
		t.Fatalf("execution result = %#v", result)
	}
	wantCalls := []string{"provision", "issue", "execute", "sync", "collect", "revoke", "destroy"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("lifecycle calls = %v, want %v", calls, wantCalls)
	}
	if _, found, _ := states.Load(context.Background(), execution.Claim.Attempt.ID); found {
		t.Fatal("terminal lifecycle state was not removed")
	}
	wantEvents := []EventPhase{EventEnvironmentProvisioned, EventCredentialsIssued, EventWorkerStarted, EventCheckpointRecorded, EventEnvironmentSynced, EventArtifactsCollected, EventCredentialsRevoked, EventEnvironmentDestroyed}
	if got := events.phases(); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("event phases = %v, want %v", got, wantEvents)
	}
	capabilities := executor.Capabilities()
	if !capabilities.Resume || !capabilities.Cancellation || !capabilities.Transcript || !capabilities.Cost || !capabilities.Artifacts || capabilities.Steering {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

func TestLifecycleExecutorRejectsLongLivedWorkerCredentials(t *testing.T) {
	t.Parallel()

	workerCalled := false
	destroyed := false
	executor, err := NewLifecycleExecutor(Options{
		Adapter: "secure", SourceDir: "/workspace",
		Provider: &fakeEnvironmentProvider{
			name: "local", capabilities: EnvironmentCapabilities{Cleanup: true},
			provision: func(context.Context, ProvisionRequest) (Environment, error) {
				return Environment{SchemaVersion: 1, ID: "env-secure", Kind: "local", RootDir: "/workspace"}, nil
			},
			destroy: func(context.Context, Environment) error { destroyed = true; return nil },
		},
		Worker: &fakeWorkerClient{name: "worker", execute: func(context.Context, WorkerRequest) (WorkerResult, error) {
			workerCalled = true
			return WorkerResult{}, nil
		}},
		CredentialBroker: &fakeCredentialBroker{issue: func(context.Context, CredentialRequest) (CredentialGrant, error) {
			return CredentialGrant{ID: "long-lived", Values: map[string]string{"API_KEY": "secret"}, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
		}},
		StateStore: newMemoryStateStore(), MaxCredentialTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("new secure executor: %v", err)
	}
	_, err = executor.Execute(context.Background(), testExecution())
	if !errors.Is(err, ErrCredentialScope) {
		t.Fatalf("execute error = %v, want ErrCredentialScope", err)
	}
	if workerCalled {
		t.Fatal("worker received a long-lived credential")
	}
	if !destroyed {
		t.Fatal("environment was not destroyed after credential rejection")
	}
}

func TestLifecycleExecutorRecoversSavedEnvironment(t *testing.T) {
	t.Parallel()

	execution := testExecution()
	states := newMemoryStateStore()
	if err := states.Save(context.Background(), LifecycleState{
		SchemaVersion: 1, AttemptID: execution.Claim.Attempt.ID, Phase: EventWorkerStarted,
		Environment: Environment{SchemaVersion: 1, ID: "env-recover", Kind: "worktree", RootDir: "/managed"},
		Checkpoint:  &WorkerCheckpoint{ID: "checkpoint-recover", ResumeToken: "resume-recover"},
	}); err != nil {
		t.Fatalf("seed lifecycle state: %v", err)
	}
	recoveredEnvironment := false
	workerRecovered := false
	provider := &fakeEnvironmentProvider{
		name: "worktree", capabilities: EnvironmentCapabilities{Recoverable: true, Snapshot: true, Cleanup: true},
		recover: func(_ context.Context, environment Environment) (Environment, error) {
			recoveredEnvironment = environment.ID == "env-recover"
			return environment, nil
		},
		sync: func(context.Context, Environment) (EnvironmentSnapshot, error) {
			return EnvironmentSnapshot{Digest: "sha256:recovered"}, nil
		},
		destroy: func(context.Context, Environment) error { return nil },
	}
	worker := &fakeWorkerClient{
		name: "native", capabilities: ExecutorCapabilities{Resume: true},
		recover: func(_ context.Context, request WorkerRequest, checkpoint *WorkerCheckpoint) (WorkerResult, bool, error) {
			workerRecovered = request.Environment.ID == "env-recover" && checkpoint != nil && checkpoint.ResumeToken == "resume-recover"
			return WorkerResult{ExecutionResult: workscheduler.ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"recovered":true}`)}}, true, nil
		},
	}
	broker := &fakeCredentialBroker{
		issue: func(context.Context, CredentialRequest) (CredentialGrant, error) {
			return CredentialGrant{ID: "grant-recovered", Values: map[string]string{"TASK_TOKEN": "replacement"}, ExpiresAt: time.Now().Add(time.Minute)}, nil
		},
		revoke: func(context.Context, CredentialGrant) error { return nil },
	}
	events := &memoryEventSink{}
	executor, err := NewLifecycleExecutor(Options{
		Adapter: "recoverable", SourceDir: "/source", Provider: provider, Worker: worker,
		CredentialBroker: broker, StateStore: states, EventSink: events,
	})
	if err != nil {
		t.Fatalf("new recoverable executor: %v", err)
	}
	result, found, err := executor.Recover(context.Background(), execution)
	if err != nil || !found || !result.Succeeded {
		t.Fatalf("recover result = %#v, found=%v, err=%v", result, found, err)
	}
	if !recoveredEnvironment || !workerRecovered {
		t.Fatalf("recovery calls environment=%v worker=%v", recoveredEnvironment, workerRecovered)
	}
	wantEvents := []EventPhase{EventRecoveryStarted, EventCredentialsIssued, EventEnvironmentSynced, EventCredentialsRevoked, EventEnvironmentDestroyed}
	if got := events.phases(); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("recovery event phases = %v, want %v", got, wantEvents)
	}
}

func testExecution() workscheduler.Execution {
	return workscheduler.Execution{
		Work: workstore.Work{ID: "work-1", WorkspaceID: "workspace-1"},
		Claim: workstore.StepClaim{
			Step:    workstore.Step{ID: "step-1", WorkID: "work-1", WorkspaceID: "workspace-1"},
			Attempt: workstore.Attempt{ID: "attempt-1", WorkID: "work-1", StepID: "step-1", WorkspaceID: "workspace-1"},
		},
	}
}

type fakeEnvironmentProvider struct {
	name         string
	capabilities EnvironmentCapabilities
	provision    func(context.Context, ProvisionRequest) (Environment, error)
	recover      func(context.Context, Environment) (Environment, error)
	sync         func(context.Context, Environment) (EnvironmentSnapshot, error)
	destroy      func(context.Context, Environment) error
}

func (provider *fakeEnvironmentProvider) Name() string { return provider.name }
func (provider *fakeEnvironmentProvider) Capabilities() EnvironmentCapabilities {
	return provider.capabilities
}
func (provider *fakeEnvironmentProvider) Provision(ctx context.Context, request ProvisionRequest) (Environment, error) {
	return provider.provision(ctx, request)
}
func (provider *fakeEnvironmentProvider) Recover(ctx context.Context, environment Environment) (Environment, error) {
	if provider.recover == nil {
		return Environment{}, ErrUnsupported
	}
	return provider.recover(ctx, environment)
}
func (provider *fakeEnvironmentProvider) Sync(ctx context.Context, environment Environment) (EnvironmentSnapshot, error) {
	if provider.sync == nil {
		return EnvironmentSnapshot{}, nil
	}
	return provider.sync(ctx, environment)
}
func (provider *fakeEnvironmentProvider) Destroy(ctx context.Context, environment Environment) error {
	if provider.destroy == nil {
		return nil
	}
	return provider.destroy(ctx, environment)
}

type fakeWorkerClient struct {
	name         string
	capabilities ExecutorCapabilities
	execute      func(context.Context, WorkerRequest) (WorkerResult, error)
	recover      func(context.Context, WorkerRequest, *WorkerCheckpoint) (WorkerResult, bool, error)
	cancel       func(context.Context, WorkerRequest) error
}

func (worker *fakeWorkerClient) Name() string                       { return worker.name }
func (worker *fakeWorkerClient) Capabilities() ExecutorCapabilities { return worker.capabilities }
func (worker *fakeWorkerClient) Execute(ctx context.Context, request WorkerRequest) (WorkerResult, error) {
	return worker.execute(ctx, request)
}
func (worker *fakeWorkerClient) Recover(ctx context.Context, request WorkerRequest, checkpoint *WorkerCheckpoint) (WorkerResult, bool, error) {
	if worker.recover == nil {
		return WorkerResult{}, false, ErrUnsupported
	}
	return worker.recover(ctx, request, checkpoint)
}
func (worker *fakeWorkerClient) Cancel(ctx context.Context, request WorkerRequest) error {
	if worker.cancel == nil {
		return ErrUnsupported
	}
	return worker.cancel(ctx, request)
}

type fakeCredentialBroker struct {
	issue  func(context.Context, CredentialRequest) (CredentialGrant, error)
	revoke func(context.Context, CredentialGrant) error
}

func (broker *fakeCredentialBroker) Issue(ctx context.Context, request CredentialRequest) (CredentialGrant, error) {
	return broker.issue(ctx, request)
}
func (broker *fakeCredentialBroker) Revoke(ctx context.Context, grant CredentialGrant) error {
	if broker.revoke == nil {
		return nil
	}
	return broker.revoke(ctx, grant)
}

type fakeArtifactCollector struct {
	collect func(context.Context, CollectRequest) ([]CollectedArtifact, error)
}

func (collector *fakeArtifactCollector) Collect(ctx context.Context, request CollectRequest) ([]CollectedArtifact, error) {
	return collector.collect(ctx, request)
}

type memoryStateStore struct {
	states map[string]LifecycleState
}

func newMemoryStateStore() *memoryStateStore {
	return &memoryStateStore{states: map[string]LifecycleState{}}
}

func (store *memoryStateStore) Save(_ context.Context, state LifecycleState) error {
	store.states[state.AttemptID] = state
	return nil
}
func (store *memoryStateStore) Load(_ context.Context, attemptID string) (LifecycleState, bool, error) {
	state, found := store.states[attemptID]
	return state, found, nil
}
func (store *memoryStateStore) Delete(_ context.Context, attemptID string) error {
	delete(store.states, attemptID)
	return nil
}

type memoryEventSink struct {
	events []LifecycleEvent
}

func (sink *memoryEventSink) Record(_ context.Context, event LifecycleEvent) error {
	sink.events = append(sink.events, event)
	return nil
}
func (sink *memoryEventSink) phases() []EventPhase {
	phases := make([]EventPhase, 0, len(sink.events))
	for _, event := range sink.events {
		phases = append(phases, event.Phase)
	}
	return phases
}
