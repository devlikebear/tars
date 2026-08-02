package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/executionplane"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

const agentRuntimeWorkAdapter = "agentruntime"

type durableSubagentContract struct {
	SchemaVersion   int               `json:"schema_version"`
	FlowID          string            `json:"flow_id"`
	Agent           string            `json:"agent"`
	ParentRunID     string            `json:"parent_run_id,omitempty"`
	RootRunID       string            `json:"root_run_id,omitempty"`
	ParentSessionID string            `json:"parent_session_id,omitempty"`
	Depth           int               `json:"depth"`
	Flow            subagentFlowInput `json:"flow"`
}

func executeDurableSubagentFlow(ctx context.Context, runtime *agentruntime.Runtime, scheduler *workscheduler.Scheduler, input subagentFlowInput) (Result, error) {
	workspaceID := serverauth.WorkspaceIDFromContext(ctx)
	meta := usage.CallMetaFromContext(ctx)
	maxThreads, maxDepth := runtime.SubagentLimits()
	agentName := strings.TrimSpace(input.Agent)
	if agentName == "" {
		agentName = "explorer"
	}
	info, ok := runtime.LookupAgent(agentName)
	if !ok {
		return JSONTextResult(map[string]any{"message": fmt.Sprintf("subagent %q is not available", agentName)}, true), nil
	}
	if message := validateSafeSubagent(info); message != "" {
		return JSONTextResult(map[string]any{"message": message}, true), nil
	}
	parentRunID, rootRunID, nextDepth, err := resolveSubagentParentContext(runtime, workspaceID, meta.RunID)
	if err != nil {
		return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
	}
	if maxDepth > 0 && nextDepth > maxDepth {
		return JSONTextResult(map[string]any{
			"message": fmt.Sprintf("subagent depth %d exceeds agentruntime_subagents_max_depth=%d", nextDepth, maxDepth),
		}, true), nil
	}
	if err := validateSubagentFlow(input.Steps, maxThreads); err != nil {
		return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
	}
	for stepIndex := range input.Steps {
		for taskIndex := range input.Steps[stepIndex].Tasks {
			task := &input.Steps[stepIndex].Tasks[taskIndex]
			normalized, message := normalizeProviderOverride(task.ProviderOverride)
			if message != "" {
				return JSONTextResult(map[string]any{"message": message}, true), nil
			}
			task.ProviderOverride = normalized
		}
	}
	flowID := strings.TrimSpace(input.FlowID)
	if flowID == "" {
		flowID = fmt.Sprintf("flow_%d", time.Now().UnixNano())
	}
	input.FlowID = flowID
	contract := durableSubagentContract{
		SchemaVersion: 1, FlowID: flowID, Agent: agentName,
		ParentRunID: parentRunID, RootRunID: rootRunID,
		ParentSessionID: strings.TrimSpace(meta.SessionID), Depth: nextDepth, Flow: input,
	}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return JSONTextResult(map[string]any{"message": fmt.Sprintf("encode durable subagent flow: %v", err)}, true), nil
	}
	policy := durableFlowPolicy(input.Policy)
	stepSpecs := durableFlowStepSpecs(input.Steps, info.Tier, policy)
	source := "subagents"
	sourceID := flowID
	if strings.TrimSpace(meta.SessionID) != "" {
		source = "session"
		sourceID = strings.TrimSpace(meta.SessionID)
	}
	work, err := scheduler.Submit(ctx, workscheduler.SubmitInput{
		WorkspaceID: workspaceID, IdempotencyKey: "subagent-flow:" + flowID,
		Kind: "subagent_flow", Source: source, SourceID: sourceID,
		CausationID: firstNonEmptyString(strings.TrimSpace(meta.RunID), flowID),
		Title:       "Subagent flow " + flowID, Objective: fmt.Sprintf("Execute %d durable subagent tasks", len(stepSpecs)),
		ContractJSON: contractJSON, Adapter: agentRuntimeWorkAdapter, ActorID: "subagents_orchestrate",
		CapabilityVersionIDs: meta.CapabilityVersionIDs, Steps: stepSpecs,
	})
	if err != nil {
		return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
	}
	accepted := map[string]any{
		"flow_id": flowID, "work_id": work.ID, "status": work.State,
		"durable": true, "task_count": len(stepSpecs), "wait_for_completion": input.WaitForCompletion,
	}
	if !input.WaitForCompletion {
		return JSONTextResult(accepted, false), nil
	}
	timeout := input.TimeoutMS
	if timeout <= 0 {
		timeout = 60000
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	projection, err := scheduler.Wait(waitCtx, work.ID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			accepted["timed_out"] = true
			accepted["message"] = "work continues in the durable scheduler"
			return JSONTextResult(accepted, false), nil
		}
		return JSONTextResult(map[string]any{"message": err.Error(), "flow_id": flowID, "work_id": work.ID}, true), nil
	}
	payload, failed := durableFlowOutput(contract, projection)
	return JSONTextResult(payload, failed), nil
}

func durableFlowPolicy(input *subagentFlowSchedulePolicy) workstore.StepSchedulePolicy {
	if input == nil {
		return workstore.StepSchedulePolicy{
			MaxAttempts: 4, RetryLimit: 1, ReplanLimit: 1, DecomposeLimit: 1,
			EscalationState: workstore.WorkStateReview,
		}
	}
	escalation := workstore.WorkState(strings.ToLower(strings.TrimSpace(input.Escalation)))
	if escalation == "" {
		escalation = workstore.WorkStateReview
	}
	return workstore.StepSchedulePolicy{
		MaxAttempts: input.MaxAttempts, RetryLimit: input.RetryLimit,
		ReplanLimit: input.ReplanLimit, DecomposeLimit: input.DecomposeLimit,
		MaxIterations: input.MaxIterations, MaxTokens: input.MaxTokens,
		MaxCostUSD: input.MaxCostUSD, EscalationState: escalation,
	}
}

func durableFlowStepSpecs(steps []subagentFlowStepInput, defaultTier string, policy workstore.StepSchedulePolicy) []workscheduler.StepSpec {
	result := make([]workscheduler.StepSpec, 0)
	previousStage := []string{}
	position := 0
	for _, stage := range steps {
		mode := strings.ToLower(strings.TrimSpace(stage.Mode))
		currentStage := make([]string, 0, len(stage.Tasks))
		for taskIndex, task := range stage.Tasks {
			position++
			key := strings.TrimSpace(task.ID)
			dependencies := append([]string(nil), task.DependsOn...)
			dependencies = append(dependencies, previousStage...)
			if mode == "sequential" && taskIndex > 0 {
				dependencies = append(dependencies, strings.TrimSpace(stage.Tasks[taskIndex-1].ID))
			}
			tier := strings.ToLower(strings.TrimSpace(task.Tier))
			if tier == "" {
				tier = strings.ToLower(strings.TrimSpace(defaultTier))
			}
			descriptionJSON, _ := json.Marshal(map[string]any{
				"flow_step_id": normalizedSubagentStepID(stage), "mode": mode,
				"prompt": task.Prompt, "tier": tier,
			})
			title := strings.TrimSpace(task.Title)
			if title == "" {
				title = key
			}
			result = append(result, workscheduler.StepSpec{
				Key: key, Title: title, Description: string(descriptionJSON), Position: position,
				DependsOn: dedupeStrings(dependencies), Policy: policy,
			})
			currentStage = append(currentStage, key)
		}
		previousStage = currentStage
	}
	return result
}

func durableFlowOutput(contract durableSubagentContract, projection workstore.WorkProjection) (map[string]any, bool) {
	stepsByKey := make(map[string]workstore.Step, len(projection.Steps))
	for _, step := range projection.Steps {
		stepsByKey[step.IdempotencyKey] = step
	}
	latestAttempts := latestAttemptsByStep(projection.Attempts)
	outputs := make([]subagentStepOutput, 0, len(contract.Flow.Steps))
	totalTasks := 0
	for _, stage := range contract.Flow.Steps {
		stageOutput := subagentStepOutput{
			ID: normalizedSubagentStepID(stage), Mode: strings.ToLower(strings.TrimSpace(stage.Mode)),
			TaskCount: len(stage.Tasks), Status: "completed", Tasks: make([]subagentTaskOutput, 0, len(stage.Tasks)),
		}
		totalTasks += len(stage.Tasks)
		for _, task := range stage.Tasks {
			step := stepsByKey[strings.TrimSpace(task.ID)]
			attempt := latestAttempts[step.ID]
			taskOutput := durableTaskOutput(contract.Agent, task, step, attempt)
			stageOutput.Tasks = append(stageOutput.Tasks, taskOutput)
			if step.State != workstore.WorkStateDone {
				stageOutput.Status = "running"
			}
			if step.State == workstore.WorkStateReview || step.State == workstore.WorkStateBlocked || step.State == workstore.WorkStateCancelled || attempt.Status == workstore.AttemptStatusFailed {
				stageOutput.Status = "failed"
				stageOutput.FailedTasks++
			}
		}
		stageOutput.Summary = buildSubagentStepSummary(stageOutput)
		outputs = append(outputs, stageOutput)
	}
	failed := projection.Work.State == workstore.WorkStateReview || projection.Work.State == workstore.WorkStateBlocked || projection.Work.State == workstore.WorkStateCancelled
	return map[string]any{
		"flow_id": contract.FlowID, "work_id": projection.Work.ID,
		"status": projection.Work.State, "durable": true, "agent": contract.Agent,
		"step_count": len(outputs), "task_count": totalTasks, "steps": outputs,
	}, failed
}

func durableTaskOutput(agent string, task subagentFlowTaskInput, step workstore.Step, attempt workstore.Attempt) subagentTaskOutput {
	var run agentruntime.Run
	if len(attempt.OutputJSON) > 0 {
		_ = json.Unmarshal(attempt.OutputJSON, &run)
	}
	if strings.TrimSpace(run.ID) != "" {
		return buildSubagentTaskOutput(strings.TrimSpace(task.ID), firstNonEmptyString(task.Title, task.ID), task.DependsOn, run)
	}
	status := string(step.State)
	if attempt.Status != "" {
		status = string(attempt.Status)
	}
	return subagentTaskOutput{
		ID: strings.TrimSpace(task.ID), Title: firstNonEmptyString(task.Title, task.ID),
		Agent: agent, Status: status, Tier: strings.TrimSpace(task.Tier),
		DependsOn: sanitizeStringList(task.DependsOn), Error: strings.TrimSpace(attempt.ErrorText),
	}
}

func latestAttemptsByStep(attempts []workstore.Attempt) map[string]workstore.Attempt {
	latest := make(map[string]workstore.Attempt)
	for _, attempt := range attempts {
		if current, ok := latest[attempt.StepID]; !ok || attempt.Number > current.Number {
			latest[attempt.StepID] = attempt
		}
	}
	return latest
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

type agentRuntimeWorkExecutor struct {
	runtime *agentruntime.Runtime
	store   *workstore.Store
}

func NewAgentRuntimeWorkExecutor(runtime *agentruntime.Runtime, stores ...*workstore.Store) workscheduler.Executor {
	var store *workstore.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &agentRuntimeWorkExecutor{runtime: runtime, store: store}
}

func (executor *agentRuntimeWorkExecutor) Adapter() string { return agentRuntimeWorkAdapter }

func (executor *agentRuntimeWorkExecutor) Execute(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, error) {
	contract, task, err := executor.resolveTask(ctx, execution)
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	projection, err := executor.runtimeProjection(ctx, execution)
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	completed := completedSubagentOutputs(projection)
	prompt, err := renderSubagentFlowPrompt(task.Prompt, completed)
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	prompt = schedulerActionPrompt(prompt, execution.Claim.Schedule.NextAction, latestAttemptError(projection, execution.Claim.Step.ID))
	tier := strings.ToLower(strings.TrimSpace(task.Tier))
	if tier == "" {
		if info, ok := executor.runtime.LookupAgent(contract.Agent); ok {
			tier = strings.ToLower(strings.TrimSpace(info.Tier))
		}
	}
	executionRoot := ""
	if environment, ok := executionplane.EnvironmentFromContext(ctx); ok {
		executionRoot = strings.TrimSpace(environment.RootDir)
	}
	run, err := subagentFlowSpawn(executor.runtime, ctx, agentruntime.SpawnRequest{
		WorkspaceID: execution.Work.WorkspaceID, WorkID: execution.Work.ID, TaskID: execution.Claim.Step.ID,
		ExecutionRoot: executionRoot,
		Title:         firstNonEmptyString(task.Title, task.ID), Prompt: prompt, Agent: contract.Agent,
		ParentRunID: contract.ParentRunID, RootRunID: contract.RootRunID,
		ParentSessionID: contract.ParentSessionID, Depth: contract.Depth,
		SessionKind: "subagent", SessionHidden: true, FlowID: contract.FlowID,
		StepID: execution.Claim.Step.ID, Tier: tier, ProviderOverride: task.ProviderOverride,
	})
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	final, err := subagentFlowWait(executor.runtime, ctx, run.ID)
	if err != nil {
		return workscheduler.ExecutionResult{}, err
	}
	return agentRunExecutionResult(final)
}

func (executor *agentRuntimeWorkExecutor) Recover(ctx context.Context, execution workscheduler.Execution) (workscheduler.ExecutionResult, bool, error) {
	run, found := executor.findRun(execution)
	if !found {
		return workscheduler.ExecutionResult{}, false, nil
	}
	if run.Status == agentruntime.RunStatusAccepted || run.Status == agentruntime.RunStatusRunning {
		final, err := subagentFlowWait(executor.runtime, ctx, run.ID)
		if err != nil {
			return workscheduler.ExecutionResult{}, true, err
		}
		run = final
	}
	result, err := agentRunExecutionResult(run)
	return result, true, err
}

func (executor *agentRuntimeWorkExecutor) Cancel(_ context.Context, execution workscheduler.Execution) error {
	run, found := executor.findRun(execution)
	if !found || run.Status == agentruntime.RunStatusCompleted || run.Status == agentruntime.RunStatusFailed || run.Status == agentruntime.RunStatusCanceled {
		return nil
	}
	_, err := executor.runtime.CancelByWorkspace(execution.Work.WorkspaceID, run.ID)
	return err
}

func (executor *agentRuntimeWorkExecutor) resolveTask(ctx context.Context, execution workscheduler.Execution) (durableSubagentContract, subagentFlowTaskInput, error) {
	if executor == nil || executor.runtime == nil {
		return durableSubagentContract{}, subagentFlowTaskInput{}, fmt.Errorf("agent runtime is not configured")
	}
	var contract durableSubagentContract
	if err := json.Unmarshal(execution.Work.ContractJSON, &contract); err != nil {
		return durableSubagentContract{}, subagentFlowTaskInput{}, fmt.Errorf("decode durable subagent contract: %w", err)
	}
	if _, ok := executor.runtime.LookupAgent(contract.Agent); !ok {
		return durableSubagentContract{}, subagentFlowTaskInput{}, fmt.Errorf("subagent %q is not available", contract.Agent)
	}
	for _, stage := range contract.Flow.Steps {
		for _, task := range stage.Tasks {
			if strings.TrimSpace(task.ID) == execution.Claim.Step.IdempotencyKey {
				return contract, task, nil
			}
		}
	}
	_ = ctx
	return durableSubagentContract{}, subagentFlowTaskInput{}, fmt.Errorf("durable subagent task %q is missing from contract", execution.Claim.Step.IdempotencyKey)
}

func (executor *agentRuntimeWorkExecutor) runtimeProjection(ctx context.Context, execution workscheduler.Execution) (workstore.WorkProjection, error) {
	if executor == nil || executor.store == nil {
		return workstore.WorkProjection{}, fmt.Errorf("work ledger is not configured")
	}
	return executor.store.GetWorkProjection(ctx, execution.Work.WorkspaceID, execution.Work.ID)
}

func (executor *agentRuntimeWorkExecutor) findRun(execution workscheduler.Execution) (agentruntime.Run, bool) {
	if executor == nil || executor.runtime == nil {
		return agentruntime.Run{}, false
	}
	for _, run := range executor.runtime.ListByWorkspace(execution.Work.WorkspaceID, 2000) {
		if strings.TrimSpace(run.TaskID) == execution.Claim.Step.ID ||
			(strings.TrimSpace(run.FlowID) == execution.Work.SourceID && strings.TrimSpace(run.StepID) == execution.Claim.Step.ID) {
			return run, true
		}
	}
	return agentruntime.Run{}, false
}

func agentRunExecutionResult(run agentruntime.Run) (workscheduler.ExecutionResult, error) {
	outputJSON, err := json.Marshal(run)
	if err != nil {
		return workscheduler.ExecutionResult{}, fmt.Errorf("encode agent runtime result: %w", err)
	}
	tokens := int64(0)
	for _, variant := range run.ConsensusVariants {
		tokens += int64(variant.TokensIn + variant.TokensOut)
	}
	result := workscheduler.ExecutionResult{
		Succeeded: run.Status == agentruntime.RunStatusCompleted, OutputJSON: outputJSON,
		Error: strings.TrimSpace(run.Error),
		Usage: workstore.StepAttemptUsage{Iterations: 1, Tokens: tokens, CostUSD: run.ConsensusCostUSD},
	}
	if !result.Succeeded && result.Error == "" {
		result.Error = string(run.Status)
	}
	return result, nil
}

func completedSubagentOutputs(projection workstore.WorkProjection) map[string]subagentCompletedTask {
	steps := make(map[string]workstore.Step, len(projection.Steps))
	for _, step := range projection.Steps {
		steps[step.ID] = step
	}
	completed := make(map[string]subagentCompletedTask)
	for _, attempt := range latestAttemptsByStep(projection.Attempts) {
		if attempt.Status != workstore.AttemptStatusSucceeded {
			continue
		}
		var run agentruntime.Run
		if err := json.Unmarshal(attempt.OutputJSON, &run); err != nil {
			continue
		}
		step := steps[attempt.StepID]
		completed[step.IdempotencyKey] = subagentCompletedTask{
			Response: strings.TrimSpace(run.Response), Summary: trimSubagentSummary(run.Response, 220), Error: strings.TrimSpace(run.Error),
		}
	}
	return completed
}

func latestAttemptError(projection workstore.WorkProjection, stepID string) string {
	latest := latestAttemptsByStep(projection.Attempts)[stepID]
	return strings.TrimSpace(latest.ErrorText)
}

func schedulerActionPrompt(prompt string, action workstore.StepExecutionAction, previousError string) string {
	contextLine := ""
	if strings.TrimSpace(previousError) != "" {
		contextLine = " Previous attempt error: " + strings.TrimSpace(previousError)
	}
	switch action {
	case workstore.StepExecutionActionRetry:
		return strings.TrimSpace(prompt + "\n\nRetry the task and correct the previous failure." + contextLine)
	case workstore.StepExecutionActionReplan:
		return strings.TrimSpace(prompt + "\n\nReassess the approach before acting, then execute a revised plan." + contextLine)
	case workstore.StepExecutionActionDecompose:
		return strings.TrimSpace(prompt + "\n\nDecompose the task into smaller verifiable actions, then complete them." + contextLine)
	default:
		return strings.TrimSpace(prompt)
	}
}
