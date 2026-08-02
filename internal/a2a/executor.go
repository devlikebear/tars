package a2a

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

const AdapterName = "a2a-http-json"

type EventKind string

const (
	EventTaskSubmitted       EventKind = "task_submitted"
	EventTaskStateObserved   EventKind = "task_state_observed"
	EventArtifactQuarantined EventKind = "artifact_quarantined"
	EventTaskCanceled        EventKind = "task_canceled"
)

type ExternalEvent struct {
	Kind              EventKind `json:"kind"`
	TaskID            string    `json:"task_id"`
	ContextID         string    `json:"context_id,omitempty"`
	State             TaskState `json:"state,omitempty"`
	AcceptedArtifacts int       `json:"accepted_artifacts,omitempty"`
	QuarantinedParts  int       `json:"quarantined_parts,omitempty"`
}

type TaskReference struct {
	TaskID    string `json:"task_id"`
	ContextID string `json:"context_id,omitempty"`
}

type Journal interface {
	Record(context.Context, workscheduler.Execution, ExternalEvent) error
	Lookup(context.Context, workscheduler.Execution) (TaskReference, bool, error)
}

type ExecutorOptions struct {
	Client          *Client
	Journal         Journal
	PollInterval    time.Duration
	MaxPollDuration time.Duration
	MaxPartBytes    int
	AcceptedModes   []string
}

type Executor struct {
	client          *Client
	journal         Journal
	pollInterval    time.Duration
	maxPollDuration time.Duration
	maxPartBytes    int
	acceptedModes   []string
}

func NewExecutor(options ExecutorOptions) (*Executor, error) {
	if options.Client == nil || options.Journal == nil {
		return nil, fmt.Errorf("a2a: client and durable journal are required")
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	maxPollDuration := options.MaxPollDuration
	if maxPollDuration <= 0 {
		maxPollDuration = 30 * time.Minute
	}
	maxPartBytes := options.MaxPartBytes
	if maxPartBytes <= 0 {
		maxPartBytes = defaultMaxArtifactPartBytes
	}
	acceptedModes := append([]string(nil), options.AcceptedModes...)
	if len(acceptedModes) == 0 {
		acceptedModes = []string{"text/plain", "application/json"}
	}
	return &Executor{
		client: options.Client, journal: options.Journal, pollInterval: pollInterval,
		maxPollDuration: maxPollDuration, maxPartBytes: maxPartBytes, acceptedModes: acceptedModes,
	}, nil
}

func (*Executor) Adapter() string {
	return AdapterName
}

func (executor *Executor) Execute(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	if executor == nil || executor.client == nil || executor.journal == nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: executor is not configured")
	}
	prompt := executionPrompt(execution)
	response, err := executor.client.SendMessage(ctx, SendMessageRequest{
		Message: Message{
			MessageID: messageIDForAttempt(execution.Claim.Attempt.ID),
			Role:      RoleUser,
			Parts:     []Part{NewTextPart(prompt)},
		},
		Configuration: SendMessageConfiguration{
			AcceptedOutputModes: append([]string(nil), executor.acceptedModes...),
			ReturnImmediately:   true,
		},
	})
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	if response.Message != nil {
		output := SanitizeMessage(*response.Message, executor.maxPartBytes)
		raw, marshalErr := json.Marshal(output)
		if marshalErr != nil {
			return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: encode direct message result: %w", marshalErr)
		}
		return workscheduler.ExecutionResult{Succeeded: true, OutputJSON: raw}, nil
	}
	task := *response.Task
	if err := executor.journal.Record(ctx, execution, ExternalEvent{
		Kind: EventTaskSubmitted, TaskID: task.ID, ContextID: task.ContextID, State: task.Status.State,
	}); err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: persist external task reference: %w", err)
	}
	return executor.awaitTask(ctx, execution, task, 1)
}

func (executor *Executor) Recover(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, bool, error) {
	if executor == nil || executor.client == nil || executor.journal == nil {
		return workscheduler.ExecutionResult{}, false, fmt.Errorf("a2a: executor is not configured")
	}
	reference, found, err := executor.journal.Lookup(ctx, execution)
	if err != nil || !found {
		return workscheduler.ExecutionResult{}, found, err
	}
	task, err := executor.client.GetTask(ctx, reference.TaskID)
	if err != nil {
		return workscheduler.ExecutionResult{}, true, err
	}
	result, err := executor.awaitTask(ctx, execution, task, 1, true)
	return result, true, err
}

func (executor *Executor) Cancel(ctx context.Context, execution workscheduler.Execution) error {
	if executor == nil || executor.client == nil || executor.journal == nil {
		return fmt.Errorf("a2a: executor is not configured")
	}
	reference, found, err := executor.journal.Lookup(ctx, execution)
	if err != nil || !found {
		return err
	}
	task, err := executor.client.CancelTask(ctx, reference.TaskID)
	if err != nil {
		return err
	}
	return executor.journal.Record(ctx, execution, ExternalEvent{
		Kind: EventTaskCanceled, TaskID: task.ID, ContextID: task.ContextID, State: task.Status.State,
	})
}

func (executor *Executor) awaitTask(
	ctx context.Context,
	execution workscheduler.Execution,
	task Task,
	iterations int,
	alreadyFetched ...bool,
) (workscheduler.ExecutionResult, error) {
	pollCtx, cancel := context.WithTimeout(ctx, executor.maxPollDuration)
	defer cancel()
	for {
		if err := executor.journal.Record(pollCtx, execution, ExternalEvent{
			Kind: EventTaskStateObserved, TaskID: task.ID, ContextID: task.ContextID, State: task.Status.State,
		}); err != nil {
			return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: persist external task status: %w", err)
		}
		if task.Status.State.Terminal() || task.Status.State.Interrupted() {
			return executor.completeTask(pollCtx, execution, task, iterations)
		}
		if len(alreadyFetched) > 0 && alreadyFetched[0] {
			alreadyFetched[0] = false
		}
		timer := time.NewTimer(executor.pollInterval)
		select {
		case <-pollCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return workscheduler.ExecutionResult{}, pollCtx.Err()
		case <-timer.C:
		}
		var err error
		task, err = executor.client.GetTask(pollCtx, task.ID)
		if err != nil {
			return workscheduler.ExecutionResult{}, err
		}
		iterations++
	}
}

func (executor *Executor) completeTask(
	ctx context.Context,
	execution workscheduler.Execution,
	task Task,
	iterations int,
) (workscheduler.ExecutionResult, error) {
	output := SanitizeTask(task, executor.maxPartBytes)
	if len(output.Quarantined) > 0 {
		if err := executor.journal.Record(ctx, execution, ExternalEvent{
			Kind: EventArtifactQuarantined, TaskID: task.ID, ContextID: task.ContextID,
			State: task.Status.State, AcceptedArtifacts: len(output.Artifacts), QuarantinedParts: len(output.Quarantined),
		}); err != nil {
			return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: persist quarantine report: %w", err)
		}
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("a2a: encode external task result: %w", err)
	}
	result := workscheduler.ExecutionResult{
		Succeeded:  task.Status.State == TaskStateCompleted,
		OutputJSON: raw,
		Usage:      workstore.StepAttemptUsage{Iterations: iterations},
	}
	switch task.Status.State {
	case TaskStateCompleted:
		return result, nil
	case TaskStateInputRequired:
		result.Error = "a2a task requires operator input"
	case TaskStateAuthRequired:
		result.Error = "a2a task requires operator authentication"
	case TaskStateFailed:
		result.Error = "a2a task failed"
	case TaskStateCanceled:
		result.Error = "a2a task was canceled"
	case TaskStateRejected:
		result.Error = "a2a task was rejected"
	default:
		result.Error = "a2a task stopped in an unsupported state"
	}
	return result, nil
}

func executionPrompt(execution workscheduler.Execution) string {
	sections := []string{}
	appendSection := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			sections = append(sections, label+": "+value)
		}
	}
	appendSection("Work", execution.Work.Title)
	appendSection("Objective", execution.Work.Objective)
	appendSection("Step", execution.Claim.Step.Title)
	appendSection("Instructions", execution.Claim.Step.Description)
	return strings.Join(sections, "\n")
}

func messageIDForAttempt(attemptID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(attemptID)))
	return "tars-" + hex.EncodeToString(digest[:16])
}

var _ workscheduler.Executor = (*Executor)(nil)
var _ workscheduler.RecoverableExecutor = (*Executor)(nil)
var _ workscheduler.CancelableExecutor = (*Executor)(nil)
