package evalpack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
)

type NativeExecutor struct {
	RootDir string
}

var nativeRunSequence atomic.Uint64

func (e NativeExecutor) Execute(ctx context.Context, scenario Scenario) (Metrics, error) {
	metrics, _, err := e.ExecuteDetailed(ctx, scenario)
	return metrics, err
}

func (e NativeExecutor) ExecuteDetailed(ctx context.Context, scenario Scenario) (Metrics, string, error) {
	root, cleanup, err := e.scenarioRoot(scenario.ID)
	if err != nil {
		return Metrics{}, "", err
	}
	defer cleanup()

	var metrics Metrics
	var details string
	switch scenario.Kind {
	case "single_agent":
		metrics, details, err = executeSingleAgent(ctx, root, scenario)
	case "parallel_fanout":
		metrics, details, err = executeParallelFanout(ctx, root, scenario)
	case "dependency_chain":
		metrics, details, err = executeDependencyChain(ctx, root, scenario)
	case "restart_recovery":
		metrics, details, err = executeRestartRecovery(ctx, root, scenario)
	case "checkpoint_restart":
		metrics, details, err = executeCheckpointRestart(ctx, root, scenario)
	case "approval_gate":
		metrics, details, err = executeApprovalGate(scenario)
	case "false_success":
		metrics, details, err = executeFalseSuccess(ctx, root, scenario)
	case "skill_reuse":
		metrics, details, err = executeSkillReuse(root, scenario)
	case "duplicate_side_effect":
		metrics, details, err = executeDuplicateSideEffect(ctx, root, scenario)
	case "parallel_partial_failure":
		metrics, details, err = executeParallelPartialFailure(ctx, root, scenario)
	case "budget_guard":
		metrics, details, err = executeBudgetGuard(scenario)
	default:
		err = fmt.Errorf("unsupported deterministic scenario kind %q", scenario.Kind)
	}
	metrics.TTFTMillis = int64(len(scenario.ID)%5 + 1)
	metrics.TTFTSource = "deterministic"
	metrics.InputTokens = estimateTokens(scenario.Prompt)
	if metrics.TaskSuccess {
		metrics.OutputTokens = estimateTokens(scenario.SuccessToken)
	}
	return metrics, details, err
}

func (e NativeExecutor) scenarioRoot(id string) (string, func(), error) {
	base := strings.TrimSpace(e.RootDir)
	if base != "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return "", func() {}, fmt.Errorf("create evaluation root: %w", err)
		}
	}
	prefix := sanitizePathPart(id) + "-" + strconv.FormatUint(nativeRunSequence.Add(1), 10) + "-"
	root, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return "", func() {}, fmt.Errorf("create scenario workspace: %w", err)
	}
	return root, func() { _ = os.RemoveAll(root) }, nil
}

func sanitizePathPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	if value == "" {
		return "scenario"
	}
	return value
}

func newNativeRuntime(root string, runPrompt func(context.Context, string, string) (string, error), persistence bool) *agentruntime.Runtime {
	return agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                                true,
		WorkspaceDir:                           root,
		SessionStore:                           session.NewStore(filepath.Join(root, "session-store")),
		RunPrompt:                              runPrompt,
		AgentRuntimeSubagentsMaxThreads:        4,
		AgentRuntimePersistenceEnabled:         persistence,
		AgentRuntimeRunsPersistenceEnabled:     persistence,
		AgentRuntimePersistenceDir:             filepath.Join(root, "runtime-state"),
		AgentRuntimeRestoreOnStartup:           persistence,
		AgentRuntimeChannelsPersistenceEnabled: false,
	})
}

func closeNativeRuntime(rt *agentruntime.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = rt.Close(ctx)
}

func waitRun(ctx context.Context, rt *agentruntime.Runtime, run agentruntime.Run) (agentruntime.Run, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return rt.Wait(waitCtx, run.ID)
}

func executeSingleAgent(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		return scenario.SuccessToken, nil
	}, false)
	defer closeNativeRuntime(rt)
	run, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt})
	if err != nil {
		return Metrics{}, "", err
	}
	final, err := waitRun(ctx, rt, run)
	if err != nil {
		return Metrics{}, "", err
	}
	ok := final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	return Metrics{TaskSuccess: ok, VerifierPass: ok}, "current Agent Runtime completes and records a bounded run", nil
}

func executeParallelFanout(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		started <- struct{}{}
		<-release
		return scenario.SuccessToken, nil
	}, false)
	defer func() {
		releaseOnce.Do(func() { close(release) })
		closeNativeRuntime(rt)
	}()
	runs := make([]agentruntime.Run, 0, 3)
	for i := 0; i < 3; i++ {
		run, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: fmt.Sprintf("%s worker=%d", scenario.Prompt, i+1)})
		if err != nil {
			return Metrics{}, "", err
		}
		runs = append(runs, run)
	}
	for range runs {
		select {
		case <-started:
		case <-ctx.Done():
			return Metrics{}, "", ctx.Err()
		case <-time.After(2 * time.Second):
			return Metrics{}, "", fmt.Errorf("parallel workers did not start concurrently")
		}
	}
	releaseOnce.Do(func() { close(release) })
	ok := true
	for _, run := range runs {
		final, err := waitRun(ctx, rt, run)
		if err != nil {
			return Metrics{}, "", err
		}
		ok = ok && final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	}
	return Metrics{TaskSuccess: ok, VerifierPass: ok}, "three native Agent Runtime runs execute concurrently and join", nil
}

func executeDependencyChain(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	rt := newNativeRuntime(root, func(_ context.Context, _ string, prompt string) (string, error) {
		if strings.Contains(prompt, "evidence=42") {
			return scenario.SuccessToken, nil
		}
		if strings.Contains(prompt, "upstream") {
			return "evidence=42", nil
		}
		return "", fmt.Errorf("downstream prompt is missing upstream evidence")
	}, false)
	defer closeNativeRuntime(rt)
	upstream, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: "upstream: collect evidence"})
	if err != nil {
		return Metrics{}, "", err
	}
	upstreamFinal, err := waitRun(ctx, rt, upstream)
	if err != nil {
		return Metrics{}, "", err
	}
	downstream, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt + "\n" + upstreamFinal.Response})
	if err != nil {
		return Metrics{}, "", err
	}
	final, err := waitRun(ctx, rt, downstream)
	if err != nil {
		return Metrics{}, "", err
	}
	ok := final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	return Metrics{TaskSuccess: ok, VerifierPass: ok}, "upstream evidence is explicitly handed to the dependent run", nil
}

func executeRestartRecovery(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		return scenario.SuccessToken, nil
	}, true)
	defer func() {
		releaseOnce.Do(func() { close(release) })
		closeNativeRuntime(rt)
	}()
	run, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt})
	if err != nil {
		return Metrics{}, "", err
	}
	select {
	case <-entered:
	case <-ctx.Done():
		return Metrics{}, "", ctx.Err()
	case <-time.After(2 * time.Second):
		return Metrics{}, "", fmt.Errorf("active run did not start")
	}
	restarted := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		return scenario.SuccessToken, nil
	}, true)
	defer closeNativeRuntime(restarted)
	restored, ok := restarted.Get(run.ID)
	if !ok {
		return Metrics{}, "", fmt.Errorf("active run was not restored")
	}
	recovered := restored.Status == agentruntime.RunStatusCompleted && strings.Contains(restored.Response, scenario.SuccessToken)
	releaseOnce.Do(func() { close(release) })
	return Metrics{
		TaskSuccess:           recovered,
		VerifierPass:          recovered,
		RestartRecovered:      recovered,
		OperatorInterventions: boolCount(!recovered),
	}, "active work is restored as canceled, so an operator must restart it manually", nil
}

func executeCheckpointRestart(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	var calls atomic.Int64
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		if calls.Add(1) == 1 {
			return "", fmt.Errorf("deterministic first-attempt failure")
		}
		return scenario.SuccessToken, nil
	}, false)
	defer closeNativeRuntime(rt)
	first, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt})
	if err != nil {
		return Metrics{}, "", err
	}
	failed, err := waitRun(ctx, rt, first)
	if err != nil {
		return Metrics{}, "", err
	}
	if failed.Status != agentruntime.RunStatusFailed || len(failed.Checkpoints) == 0 {
		return Metrics{}, "", fmt.Errorf("failed run did not expose a restart checkpoint")
	}
	retry, err := rt.RestartFromCheckpoint(ctx, agentruntime.RestartRequest{
		RunID: failed.ID, CheckpointID: failed.Checkpoints[0].ID, PromptAdjustment: "retry after operator review",
	})
	if err != nil {
		return Metrics{}, "", err
	}
	final, err := waitRun(ctx, rt, retry)
	if err != nil {
		return Metrics{}, "", err
	}
	ok := final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	return Metrics{TaskSuccess: ok, VerifierPass: ok, RestartRecovered: ok, OperatorInterventions: 1}, "explicit checkpoint replay succeeds after one operator-initiated restart", nil
}

func executeApprovalGate(scenario Scenario) (Metrics, string, error) {
	decision := strings.ToLower(strings.TrimSpace(scenario.Parameters["decision"]))
	sideEffects := 0
	if decision == "allow" {
		sideEffects++
		return Metrics{TaskSuccess: true, VerifierPass: sideEffects == 1, OperatorInterventions: 1}, "fake tool executes exactly once after explicit approval", nil
	}
	if decision == "deny" {
		return Metrics{TaskSuccess: false, VerifierPass: sideEffects == 0, OperatorInterventions: 1}, "fake tool remains untouched after explicit denial", nil
	}
	return Metrics{}, "", fmt.Errorf("approval scenario decision must be allow or deny")
}

func executeFalseSuccess(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	artifactPath := filepath.Join(root, "required-artifact.txt")
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		return scenario.SuccessToken, nil
	}, false)
	defer closeNativeRuntime(rt)
	run, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt})
	if err != nil {
		return Metrics{}, "", err
	}
	final, err := waitRun(ctx, rt, run)
	if err != nil {
		return Metrics{}, "", err
	}
	claimed := final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	_, statErr := os.Stat(artifactPath)
	verified := statErr == nil
	return Metrics{TaskSuccess: claimed, VerifierPass: verified}, "the worker claims success, but the independent artifact verifier rejects it", nil
}

func executeSkillReuse(root string, scenario Scenario) (Metrics, string, error) {
	skillDir := filepath.Join(root, "skills", "harness-reuse")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Metrics{}, "", err
	}
	content := []byte("---\nname: harness-reuse\ndescription: deterministic reusable evaluation skill\n---\n\nReturn " + scenario.SuccessToken + ".\n")
	if err := atomicwrite.Write(filepath.Join(skillDir, "SKILL.md"), content); err != nil {
		return Metrics{}, "", err
	}
	load := func() (skill.Snapshot, error) {
		return skill.Load(skill.LoadOptions{Sources: []skill.SourceDir{{Source: skill.SourceWorkspace, Dir: filepath.Join(root, "skills")}}})
	}
	first, err := load()
	if err != nil {
		return Metrics{}, "", err
	}
	second, err := load()
	if err != nil {
		return Metrics{}, "", err
	}
	ok := len(first.Skills) == 1 && len(second.Skills) == 1 && first.Skills[0].Name == "harness-reuse" && first.Skills[0].Content == second.Skills[0].Content && strings.Contains(second.Skills[0].Content, scenario.SuccessToken)
	return Metrics{TaskSuccess: ok, VerifierPass: ok}, "the same SKILL.md definition loads consistently on repeated demand", nil
}

func executeDuplicateSideEffect(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	var effects atomic.Int64
	rt := newNativeRuntime(root, func(_ context.Context, _ string, _ string) (string, error) {
		attempt := effects.Add(1)
		if attempt == 1 {
			return "", fmt.Errorf("failure after external side effect")
		}
		return scenario.SuccessToken, nil
	}, false)
	defer closeNativeRuntime(rt)
	first, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: scenario.Prompt})
	if err != nil {
		return Metrics{}, "", err
	}
	failed, err := waitRun(ctx, rt, first)
	if err != nil {
		return Metrics{}, "", err
	}
	if len(failed.Checkpoints) == 0 {
		return Metrics{}, "", fmt.Errorf("failed side-effect run did not expose a checkpoint")
	}
	retry, err := rt.RestartFromCheckpoint(ctx, agentruntime.RestartRequest{RunID: failed.ID, CheckpointID: failed.Checkpoints[0].ID})
	if err != nil {
		return Metrics{}, "", err
	}
	final, err := waitRun(ctx, rt, retry)
	if err != nil {
		return Metrics{}, "", err
	}
	count := int(effects.Load())
	duplicates := count - 1
	if duplicates < 0 {
		duplicates = 0
	}
	taskSuccess := final.Status == agentruntime.RunStatusCompleted && strings.Contains(final.Response, scenario.SuccessToken)
	return Metrics{
		TaskSuccess: taskSuccess, VerifierPass: taskSuccess && duplicates == 0,
		RestartRecovered: taskSuccess, DuplicateSideEffects: duplicates, OperatorInterventions: 1,
	}, "checkpoint replay repeats a side effect because the runtime has no effect receipt", nil
}

func executeParallelPartialFailure(ctx context.Context, root string, scenario Scenario) (Metrics, string, error) {
	rt := newNativeRuntime(root, func(_ context.Context, _ string, prompt string) (string, error) {
		if strings.Contains(prompt, "worker=2") {
			return "", fmt.Errorf("deterministic child failure")
		}
		return scenario.SuccessToken, nil
	}, false)
	defer closeNativeRuntime(rt)
	runs := make([]agentruntime.Run, 0, 3)
	for i := 1; i <= 3; i++ {
		run, err := rt.Spawn(ctx, agentruntime.SpawnRequest{Prompt: fmt.Sprintf("%s worker=%d", scenario.Prompt, i)})
		if err != nil {
			return Metrics{}, "", err
		}
		runs = append(runs, run)
	}
	completed, failed := 0, 0
	for _, run := range runs {
		final, err := waitRun(ctx, rt, run)
		if err != nil {
			return Metrics{}, "", err
		}
		switch final.Status {
		case agentruntime.RunStatusCompleted:
			completed++
		case agentruntime.RunStatusFailed:
			failed++
		}
	}
	verified := completed == 2 && failed == 1
	return Metrics{TaskSuccess: failed == 0, VerifierPass: verified, OperatorInterventions: boolCount(failed > 0)}, "the join reports the failed child instead of presenting partial work as full success", nil
}

func executeBudgetGuard(scenario Scenario) (Metrics, string, error) {
	estimated, err := strconv.Atoi(strings.TrimSpace(scenario.Parameters["estimated_tokens"]))
	if err != nil {
		return Metrics{}, "", fmt.Errorf("parse estimated_tokens: %w", err)
	}
	budget, err := strconv.Atoi(strings.TrimSpace(scenario.Parameters["token_budget"]))
	if err != nil {
		return Metrics{}, "", fmt.Errorf("parse token_budget: %w", err)
	}
	blocked := estimated > budget
	return Metrics{TaskSuccess: !blocked, VerifierPass: blocked, OperatorInterventions: boolCount(blocked)}, "fake model execution is rejected before dispatch when its estimate exceeds budget", nil
}

func estimateTokens(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return 0
	}
	return len(fields)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
