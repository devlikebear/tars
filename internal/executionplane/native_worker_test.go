package executionplane

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
)

func TestSchedulerWorkerClientDelegatesWithExecutionEnvironment(t *testing.T) {
	t.Parallel()

	execution := testExecution()
	environment := Environment{SchemaVersion: 1, ID: "worktree:attempt-1", Kind: "managed-worktree", RootDir: t.TempDir()}
	delegated := &recordingSchedulerExecutor{adapter: "agentruntime"}
	worker, err := NewSchedulerWorkerClient("native-agentruntime", delegated, true)
	if err != nil {
		t.Fatalf("new scheduler worker: %v", err)
	}
	result, err := worker.Execute(context.Background(), WorkerRequest{Execution: execution, Environment: environment})
	if err != nil {
		t.Fatalf("execute scheduler worker: %v", err)
	}
	if !result.ExecutionResult.Succeeded || string(result.ExecutionResult.OutputJSON) != `{"native":true}` {
		t.Fatalf("worker result = %#v", result)
	}
	if delegated.environment.ID != environment.ID || delegated.environment.RootDir != environment.RootDir {
		t.Fatalf("delegated environment = %#v", delegated.environment)
	}
	capabilities := worker.Capabilities()
	if !capabilities.Resume || !capabilities.Cancellation || !capabilities.ToolPolicy || !capabilities.Cost || capabilities.Steering || capabilities.Transcript {
		t.Fatalf("native capabilities = %#v", capabilities)
	}

	recovered, found, err := worker.Recover(context.Background(), WorkerRequest{Execution: execution, Environment: environment}, &WorkerCheckpoint{ID: "native"})
	if err != nil || !found || !recovered.ExecutionResult.Succeeded || delegated.recoverCalls != 1 {
		t.Fatalf("recover result=%#v found=%v calls=%d err=%v", recovered, found, delegated.recoverCalls, err)
	}
	if err := worker.Cancel(context.Background(), WorkerRequest{Execution: execution, Environment: environment}); err != nil || delegated.cancelCalls != 1 {
		t.Fatalf("cancel calls=%d err=%v", delegated.cancelCalls, err)
	}
}

func TestSchedulerWorkerClientReportsUnsupportedOperations(t *testing.T) {
	t.Parallel()

	worker, err := NewSchedulerWorkerClient("native", executeOnlyScheduler{adapter: "native"}, false)
	if err != nil {
		t.Fatalf("new execute-only worker: %v", err)
	}
	if worker.Capabilities().Resume || worker.Capabilities().Cancellation || worker.Capabilities().ToolPolicy {
		t.Fatalf("execute-only capabilities = %#v", worker.Capabilities())
	}
	request := WorkerRequest{Execution: testExecution(), Environment: Environment{RootDir: t.TempDir()}}
	if _, found, err := worker.Recover(context.Background(), request, nil); found || err == nil {
		t.Fatalf("unsupported recover found=%v err=%v", found, err)
	}
	if err := worker.Cancel(context.Background(), request); err == nil {
		t.Fatal("unsupported cancel succeeded")
	}
}

type executeOnlyScheduler struct{ adapter string }

func (executor executeOnlyScheduler) Adapter() string { return executor.adapter }
func (executeOnlyScheduler) Execute(context.Context, workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	return workscheduler.ExecutionResult{Succeeded: true}, nil
}

type recordingSchedulerExecutor struct {
	adapter      string
	environment  Environment
	recoverCalls int
	cancelCalls  int
}

func (executor *recordingSchedulerExecutor) Adapter() string { return executor.adapter }
func (executor *recordingSchedulerExecutor) Execute(ctx context.Context, _ workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	executor.environment, _ = EnvironmentFromContext(ctx)
	return workscheduler.ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"native":true}`)}, nil
}
func (executor *recordingSchedulerExecutor) Recover(ctx context.Context, _ workscheduler.Execution) (workscheduler.ExecutionResult, bool, error) {
	executor.recoverCalls++
	executor.environment, _ = EnvironmentFromContext(ctx)
	return workscheduler.ExecutionResult{Succeeded: true}, true, nil
}
func (executor *recordingSchedulerExecutor) Cancel(ctx context.Context, _ workscheduler.Execution) error {
	executor.cancelCalls++
	executor.environment, _ = EnvironmentFromContext(ctx)
	return nil
}
