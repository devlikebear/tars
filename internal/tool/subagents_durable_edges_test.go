package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestDurableFlowPolicyAndDAGProjectionContracts(t *testing.T) {
	t.Parallel()

	defaults := durableFlowPolicy(nil)
	if defaults.MaxAttempts != 4 || defaults.RetryLimit != 1 || defaults.ReplanLimit != 1 || defaults.DecomposeLimit != 1 || defaults.EscalationState != workstore.WorkStateReview {
		t.Fatalf("default durable policy=%+v", defaults)
	}
	custom := durableFlowPolicy(&subagentFlowSchedulePolicy{
		MaxAttempts: 6, RetryLimit: 2, ReplanLimit: 1, DecomposeLimit: 2,
		MaxIterations: 9, MaxTokens: 1000, MaxCostUSD: 0.5, Escalation: " BLOCKED ",
	})
	if custom.MaxAttempts != 6 || custom.EscalationState != workstore.WorkStateBlocked || custom.MaxCostUSD != 0.5 {
		t.Fatalf("custom durable policy=%+v", custom)
	}

	steps := []subagentFlowStepInput{
		{ID: "research", Mode: "parallel", Tasks: []subagentFlowTaskInput{
			{ID: "backend", Title: "Backend", Prompt: "inspect backend"},
			{ID: "docs", Prompt: "inspect docs", Tier: " HIGH ", DependsOn: []string{"external", "external", " "}},
		}},
		{ID: "report", Mode: "sequential", Tasks: []subagentFlowTaskInput{
			{ID: "draft", Prompt: "draft"},
			{ID: "publish", Prompt: "publish", DependsOn: []string{"backend"}},
		}},
	}
	specs := durableFlowStepSpecs(steps, "medium", custom)
	if len(specs) != 4 || specs[0].Position != 1 || specs[3].Position != 4 || specs[1].Title != "docs" {
		t.Fatalf("durable DAG specs=%+v", specs)
	}
	if strings.Join(specs[2].DependsOn, ",") != "backend,docs" || strings.Join(specs[3].DependsOn, ",") != "backend,docs,draft" {
		t.Fatalf("durable DAG dependencies draft=%v publish=%v", specs[2].DependsOn, specs[3].DependsOn)
	}
	var description struct {
		Tier string `json:"tier"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(specs[0].Description), &description); err != nil || description.Tier != "medium" || description.Mode != "parallel" {
		t.Fatalf("durable description=%+v err=%v", description, err)
	}
	if got := dedupeStrings([]string{" a ", "", "a", "b"}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("deduped strings=%v", got)
	}
	if firstNonEmptyString(" ", " value ", "later") != "value" || firstNonEmptyString("", " ") != "" {
		t.Fatal("firstNonEmptyString did not normalize values")
	}
}

func TestDurableFlowOutputPreservesLatestAttemptAndFailureState(t *testing.T) {
	t.Parallel()

	contract := durableSubagentContract{
		FlowID: "flow-output", Agent: "explorer",
		Flow: subagentFlowInput{Steps: []subagentFlowStepInput{
			{ID: "stage", Mode: "sequential", Tasks: []subagentFlowTaskInput{
				{ID: "done", Title: "Done", Tier: "medium"},
				{ID: "failed", DependsOn: []string{"done"}},
			}},
		}},
	}
	runRaw, _ := json.Marshal(agentruntime.Run{
		ID: "run-done", Status: agentruntime.RunStatusCompleted, Response: "verified output",
		ConsensusCostUSD: 0.25,
	})
	projection := workstore.WorkProjection{
		Work: workstore.Work{ID: "work-output", State: workstore.WorkStateReview},
		Steps: []workstore.Step{
			{ID: "step-done", IdempotencyKey: "done", State: workstore.WorkStateDone},
			{ID: "step-failed", IdempotencyKey: "failed", State: workstore.WorkStateReview},
		},
		Attempts: []workstore.Attempt{
			{ID: "old", StepID: "step-done", Number: 1, Status: workstore.AttemptStatusFailed, ErrorText: "old"},
			{ID: "latest", StepID: "step-done", Number: 2, Status: workstore.AttemptStatusSucceeded, OutputJSON: runRaw},
			{ID: "failed", StepID: "step-failed", Number: 1, Status: workstore.AttemptStatusFailed, ErrorText: "proof failed"},
		},
	}
	payload, failed := durableFlowOutput(contract, projection)
	if !failed || payload["flow_id"] != "flow-output" || payload["task_count"] != 2 {
		t.Fatalf("durable output payload=%+v failed=%v", payload, failed)
	}
	outputs := payload["steps"].([]subagentStepOutput)
	if len(outputs) != 1 || outputs[0].Status != "failed" || outputs[0].FailedTasks != 1 || outputs[0].Tasks[0].Response != "verified output" || outputs[0].Tasks[1].Error != "proof failed" {
		t.Fatalf("durable stage output=%+v", outputs)
	}
	latest := latestAttemptsByStep(projection.Attempts)
	if latest["step-done"].ID != "latest" || latestAttemptError(projection, "step-failed") != "proof failed" {
		t.Fatalf("latest attempts=%+v", latest)
	}
	completed := completedSubagentOutputs(projection)
	if completed["done"].Response != "verified output" {
		t.Fatalf("completed subagent outputs=%+v", completed)
	}
}

func TestAgentRuntimeWorkExecutorRejectsUnboundContracts(t *testing.T) {
	t.Parallel()

	executor := NewAgentRuntimeWorkExecutor(nil).(*agentRuntimeWorkExecutor)
	if executor.Adapter() != agentRuntimeWorkAdapter {
		t.Fatalf("adapter=%q", executor.Adapter())
	}
	execution := workscheduler.Execution{
		Work:  workstore.Work{WorkspaceID: "workspace", ContractJSON: json.RawMessage(`{`)},
		Claim: workstore.StepClaim{Step: workstore.Step{ID: "step", IdempotencyKey: "task"}},
	}
	if _, _, err := executor.resolveTask(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "runtime is not configured") {
		t.Fatalf("nil runtime resolve error=%v", err)
	}
	if _, err := executor.runtimeProjection(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "ledger is not configured") {
		t.Fatalf("nil ledger projection error=%v", err)
	}
	if _, found := executor.findRun(execution); found {
		t.Fatal("nil runtime found a run")
	}
	if _, found, err := executor.Recover(context.Background(), execution); err != nil || found {
		t.Fatalf("nil runtime Recover() found=%v err=%v", found, err)
	}
	if err := executor.Cancel(context.Background(), execution); err != nil {
		t.Fatalf("nil runtime Cancel(): %v", err)
	}

	runtime, _ := newAgentRuntimeForSubagentToolTests(t, 2, 1, func(context.Context, string, string, []string, string) (string, error) {
		return "ok", nil
	})
	executor.runtime = runtime
	if _, _, err := executor.resolveTask(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "decode durable subagent contract") {
		t.Fatalf("invalid contract error=%v", err)
	}
	missingAgent := durableSubagentContract{Agent: "missing", Flow: subagentFlowInput{Steps: []subagentFlowStepInput{{Tasks: []subagentFlowTaskInput{{ID: "task"}}}}}}
	execution.Work.ContractJSON, _ = json.Marshal(missingAgent)
	if _, _, err := executor.resolveTask(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "is not available") {
		t.Fatalf("missing agent error=%v", err)
	}
	bound := missingAgent
	bound.Agent = "explorer"
	execution.Work.ContractJSON, _ = json.Marshal(bound)
	contract, task, err := executor.resolveTask(context.Background(), execution)
	if err != nil || contract.Agent != "explorer" || task.ID != "task" {
		t.Fatalf("bound contract=%+v task=%+v err=%v", contract, task, err)
	}
	execution.Claim.Step.IdempotencyKey = "missing"
	if _, _, err := executor.resolveTask(context.Background(), execution); err == nil || !strings.Contains(err.Error(), "missing from contract") {
		t.Fatalf("unbound task error=%v", err)
	}
}

func TestAgentRunResultAndSchedulerActionContracts(t *testing.T) {
	t.Parallel()

	completed := agentruntime.Run{
		ID: "run", Status: agentruntime.RunStatusCompleted, ConsensusCostUSD: 0.4,
		ConsensusVariants: []agentruntime.ConsensusVariantRecord{{TokensIn: 10, TokensOut: 5}, {TokensIn: 2, TokensOut: 3}},
	}
	result, err := agentRunExecutionResult(completed)
	if err != nil || !result.Succeeded || result.Usage.Tokens != 20 || result.Usage.CostUSD != 0.4 || result.Usage.Iterations != 1 {
		t.Fatalf("completed execution result=%+v err=%v", result, err)
	}
	failed := completed
	failed.Status = agentruntime.RunStatusFailed
	failed.Error = ""
	result, err = agentRunExecutionResult(failed)
	if err != nil || result.Succeeded || result.Error != string(agentruntime.RunStatusFailed) {
		t.Fatalf("failed execution result=%+v err=%v", result, err)
	}

	for action, phrase := range map[workstore.StepExecutionAction]string{
		workstore.StepExecutionActionRetry:     "Retry the task",
		workstore.StepExecutionActionReplan:    "Reassess the approach",
		workstore.StepExecutionActionDecompose: "Decompose the task",
	} {
		prompt := schedulerActionPrompt(" original ", action, " previous error ")
		if !strings.Contains(prompt, phrase) || !strings.Contains(prompt, "previous error") {
			t.Errorf("action %s prompt=%q", action, prompt)
		}
	}
	if got := schedulerActionPrompt(" original ", workstore.StepExecutionActionExecute, "ignored"); got != "original" {
		t.Fatalf("execute action prompt=%q", got)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "workspace")
	runtime, _ := newAgentRuntimeForSubagentToolTests(t, 2, 1, func(context.Context, string, string, []string, string) (string, error) {
		return "ok", nil
	})
	resultTool, err := executeDurableSubagentFlow(ctx, runtime, nil, subagentFlowInput{Agent: "missing"})
	if err != nil || !resultTool.IsError || !strings.Contains(resultTool.Text(), "is not available") {
		t.Fatalf("unavailable durable agent result=%s err=%v", resultTool.Text(), err)
	}
}
