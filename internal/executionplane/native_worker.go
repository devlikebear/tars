package executionplane

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/workscheduler"
)

type executionEnvironmentContextKey struct{}

func WithEnvironment(ctx context.Context, environment Environment) context.Context {
	return context.WithValue(ctx, executionEnvironmentContextKey{}, environment)
}

func EnvironmentFromContext(ctx context.Context) (Environment, bool) {
	if ctx == nil {
		return Environment{}, false
	}
	environment, ok := ctx.Value(executionEnvironmentContextKey{}).(Environment)
	return environment, ok
}

// SchedulerWorkerClient lets existing native scheduler executors participate
// in the execution-plane lifecycle without changing their durable result
// contract. The environment is carried through context for adapters that can
// honor an isolated working directory.
type SchedulerWorkerClient struct {
	name       string
	executor   workscheduler.Executor
	recoverer  workscheduler.RecoverableExecutor
	canceller  workscheduler.CancelableExecutor
	toolPolicy bool
}

func NewSchedulerWorkerClient(name string, executor workscheduler.Executor, toolPolicy bool) (*SchedulerWorkerClient, error) {
	name = strings.TrimSpace(name)
	if name == "" || executor == nil || strings.TrimSpace(executor.Adapter()) == "" {
		return nil, fmt.Errorf("executionplane: native worker name and scheduler executor are required")
	}
	worker := &SchedulerWorkerClient{name: name, executor: executor, toolPolicy: toolPolicy}
	worker.recoverer, _ = executor.(workscheduler.RecoverableExecutor)
	worker.canceller, _ = executor.(workscheduler.CancelableExecutor)
	return worker, nil
}

func (worker *SchedulerWorkerClient) Name() string { return worker.name }

func (worker *SchedulerWorkerClient) Capabilities() ExecutorCapabilities {
	if worker == nil {
		return ExecutorCapabilities{}
	}
	return ExecutorCapabilities{
		Resume: worker.recoverer != nil, Cancellation: worker.canceller != nil,
		ToolPolicy: worker.toolPolicy, Cost: true,
	}
}

func (worker *SchedulerWorkerClient) Execute(ctx context.Context, request WorkerRequest) (WorkerResult, error) {
	if worker == nil || worker.executor == nil {
		return WorkerResult{}, fmt.Errorf("executionplane: native worker is not configured")
	}
	result, err := worker.executor.Execute(WithEnvironment(ctx, request.Environment), request.Execution)
	return WorkerResult{ExecutionResult: result}, err
}

func (worker *SchedulerWorkerClient) Recover(ctx context.Context, request WorkerRequest, _ *WorkerCheckpoint) (WorkerResult, bool, error) {
	if worker == nil || worker.recoverer == nil {
		return WorkerResult{}, false, fmt.Errorf("%w: native worker cannot resume", ErrUnsupported)
	}
	result, found, err := worker.recoverer.Recover(WithEnvironment(ctx, request.Environment), request.Execution)
	return WorkerResult{ExecutionResult: result}, found, err
}

func (worker *SchedulerWorkerClient) Cancel(ctx context.Context, request WorkerRequest) error {
	if worker == nil || worker.canceller == nil {
		return fmt.Errorf("%w: native worker cannot cancel", ErrUnsupported)
	}
	return worker.canceller.Cancel(WithEnvironment(ctx, request.Environment), request.Execution)
}

var _ WorkerClient = (*SchedulerWorkerClient)(nil)
