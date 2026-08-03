package executionplane

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
)

func TestOSCommandRunnerPreservesOutputExitAndEnvironmentBoundary(t *testing.T) {
	t.Parallel()

	runner := OSCommandRunner{}
	result, err := runner.Run(context.Background(), CommandSpec{
		Command: "/bin/sh", Args: []string{"-c", "read value; printf '%s:%s' \"$BOUND\" \"$value\"; printf warning >&2"},
		Env: []string{"BOUND=isolated", "PATH=/usr/bin:/bin"}, Stdin: []byte("input\n"),
	})
	if err != nil || result.ExitCode != 0 || result.Stdout != "isolated:input" || result.Stderr != "warning" {
		t.Fatalf("command result = %+v err=%v", result, err)
	}
	result, err = runner.Run(context.Background(), CommandSpec{
		Command: "/bin/sh", Args: []string{"-c", "printf failed >&2; exit 9"}, InheritEnv: true,
	})
	if err == nil || result.ExitCode != 9 || result.Stderr != "failed" {
		t.Fatalf("failed command result = %+v err=%v", result, err)
	}
}

func TestLifecycleExecutorDescriptorCancellationAndCredentialValidation(t *testing.T) {
	t.Parallel()

	if (*LifecycleExecutor)(nil).Adapter() != "" || (*LifecycleExecutor)(nil).Capabilities() != (ExecutorCapabilities{}) ||
		(*LifecycleExecutor)(nil).Descriptor() != (AdapterDescriptor{}) {
		t.Fatal("nil lifecycle executor accessors are not safe")
	}
	if _, err := (*LifecycleExecutor)(nil).Execute(context.Background(), testExecution()); err == nil {
		t.Fatal("nil lifecycle executor executed")
	}
	if _, _, err := (*LifecycleExecutor)(nil).Recover(context.Background(), testExecution()); err == nil {
		t.Fatal("nil lifecycle executor recovered")
	}
	if err := (*LifecycleExecutor)(nil).Cancel(context.Background(), testExecution()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("nil cancellation error = %v", err)
	}

	invalid := []Options{
		{},
		{Adapter: "adapter"},
		{Adapter: "adapter", Provider: &fakeEnvironmentProvider{}},
		{Adapter: "adapter", Provider: &fakeEnvironmentProvider{name: "provider"}},
		{Adapter: "adapter", Provider: &fakeEnvironmentProvider{name: "provider"}, Worker: &fakeWorkerClient{name: "worker"}},
	}
	for index, options := range invalid {
		if _, err := NewLifecycleExecutor(options); err == nil {
			t.Fatalf("invalid lifecycle options %d accepted: %+v", index, options)
		}
	}

	execution := testExecution()
	states := newMemoryStateStore()
	if err := states.Save(context.Background(), LifecycleState{
		SchemaVersion: 1, AttemptID: execution.Claim.Attempt.ID, Phase: EventWorkerStarted,
		Environment:  Environment{SchemaVersion: 1, ID: "env-cancel", Kind: "managed", RootDir: "/managed"},
		CredentialID: "grant-cancel",
	}); err != nil {
		t.Fatal(err)
	}
	var calls []string
	provider := &fakeEnvironmentProvider{
		name: "managed", capabilities: EnvironmentCapabilities{Recoverable: true, Cleanup: true},
		recover: func(_ context.Context, environment Environment) (Environment, error) {
			calls = append(calls, "recover")
			return environment, nil
		},
		destroy: func(context.Context, Environment) error {
			calls = append(calls, "destroy")
			return nil
		},
	}
	worker := &fakeWorkerClient{name: "worker", capabilities: ExecutorCapabilities{Cancellation: true}}
	worker.cancel = func(context.Context, WorkerRequest) error {
		calls = append(calls, "cancel")
		return nil
	}
	broker := &fakeCredentialBroker{revoke: func(context.Context, CredentialGrant) error {
		calls = append(calls, "revoke")
		return nil
	}}
	events := &memoryEventSink{}
	executor, err := NewLifecycleExecutor(Options{
		Adapter: "cancel-adapter", SourceDir: "/source", Provider: provider, Worker: worker,
		CredentialBroker: broker, StateStore: states, EventSink: events,
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := executor.Descriptor()
	if executor.Adapter() != "cancel-adapter" || descriptor.Provider != "managed" || descriptor.Worker != "worker" || !descriptor.Executor.Cancellation {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	if err := executor.Cancel(context.Background(), execution); err != nil {
		t.Fatalf("cancel lifecycle: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"recover", "cancel", "revoke", "destroy"}) {
		t.Fatalf("cancel calls = %v", calls)
	}
	if _, found, err := states.Load(context.Background(), execution.Claim.Attempt.ID); err != nil || found {
		t.Fatalf("cancel state found=%v err=%v", found, err)
	}
	if got := events.phases(); !reflect.DeepEqual(got, []EventPhase{EventWorkerCancelled, EventCredentialsRevoked, EventEnvironmentDestroyed}) {
		t.Fatalf("cancel events = %v", got)
	}
	if err := executor.Cancel(context.Background(), execution); err != nil {
		t.Fatalf("cancel missing state = %v", err)
	}

	credentialCases := []CredentialGrant{
		{ID: "id-only"},
		{Values: map[string]string{"TOKEN": "value"}, ExpiresAt: time.Now().Add(time.Minute)},
		{ID: "expired", Values: map[string]string{"TOKEN": "value"}, ExpiresAt: time.Now().Add(-time.Minute)},
		{ID: "long", Values: map[string]string{"TOKEN": "value"}, ExpiresAt: time.Now().Add(24 * time.Hour)},
		{ID: "empty-key", Values: map[string]string{"": "value"}, ExpiresAt: time.Now().Add(time.Minute)},
		{ID: "empty-value", Values: map[string]string{"TOKEN": ""}, ExpiresAt: time.Now().Add(time.Minute)},
	}
	for index, grant := range credentialCases {
		if err := executor.validateCredentialGrant(grant); !errors.Is(err, ErrCredentialScope) {
			t.Fatalf("credential case %d error = %v", index, err)
		}
	}
	if err := executor.validateCredentialGrant(CredentialGrant{}); err != nil {
		t.Fatalf("empty credential grant = %v", err)
	}
}

func TestLifecycleExecutorFailsClosedAcrossWorkerFinalizationAndCleanup(t *testing.T) {
	t.Parallel()

	execution := testExecution()
	states := newMemoryStateStore()
	provider := &fakeEnvironmentProvider{
		name: "failing-provider", capabilities: EnvironmentCapabilities{Snapshot: true, Cleanup: true},
		provision: func(context.Context, ProvisionRequest) (Environment, error) {
			return Environment{SchemaVersion: 1, ID: "env-failure", Kind: "managed", RootDir: "/managed"}, nil
		},
		sync: func(context.Context, Environment) (EnvironmentSnapshot, error) {
			return EnvironmentSnapshot{}, errors.New("sync failed")
		},
		destroy: func(context.Context, Environment) error { return errors.New("destroy failed") },
	}
	worker := &fakeWorkerClient{name: "failing-worker", execute: func(context.Context, WorkerRequest) (WorkerResult, error) {
		return WorkerResult{
			ExecutionResult: workscheduler.ExecutionResult{OutputJSON: []byte(`{"partial":true}`)},
			Checkpoint:      &WorkerCheckpoint{ID: "checkpoint-failure"},
		}, errors.New("worker failed")
	}}
	broker := &fakeCredentialBroker{
		issue: func(context.Context, CredentialRequest) (CredentialGrant, error) {
			return CredentialGrant{ID: "grant-failure", Values: map[string]string{"TOKEN": "secret"}, ExpiresAt: time.Now().Add(time.Minute)}, nil
		},
		revoke: func(context.Context, CredentialGrant) error { return errors.New("revoke failed") },
	}
	collector := &fakeArtifactCollector{collect: func(context.Context, CollectRequest) ([]CollectedArtifact, error) {
		return nil, errors.New("collect failed")
	}}
	executor, err := NewLifecycleExecutor(Options{
		Adapter: "failure-adapter", SourceDir: "/source", Provider: provider, Worker: worker,
		CredentialBroker: broker, ArtifactCollector: collector, StateStore: states,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), execution)
	if err == nil || !strings.Contains(err.Error(), "worker failed") || !strings.Contains(err.Error(), "sync failed") ||
		!strings.Contains(err.Error(), "collect failed") || !strings.Contains(err.Error(), "revoke failed") || !strings.Contains(err.Error(), "destroy failed") {
		t.Fatalf("joined failure result=%+v err=%v", result, err)
	}
	if _, found, loadErr := states.Load(context.Background(), execution.Claim.Attempt.ID); loadErr != nil || !found {
		t.Fatalf("failed cleanup state found=%v err=%v", found, loadErr)
	}

	destroyed := false
	invalidEnvironmentExecutor, err := NewLifecycleExecutor(Options{
		Adapter: "invalid-environment", SourceDir: "/source",
		Provider: &fakeEnvironmentProvider{
			name: "bad", provision: func(context.Context, ProvisionRequest) (Environment, error) {
				return Environment{ID: "missing-fields"}, nil
			},
			destroy: func(context.Context, Environment) error { destroyed = true; return nil },
		},
		Worker: &fakeWorkerClient{name: "worker"}, StateStore: newMemoryStateStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalidEnvironmentExecutor.Execute(context.Background(), execution); err == nil || !destroyed {
		t.Fatalf("invalid environment error=%v destroyed=%v", err, destroyed)
	}
}
