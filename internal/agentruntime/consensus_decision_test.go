package agentruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestAutomaticConsensusRequiresBaselineExpectedBenefitAndReason(t *testing.T) {
	t.Parallel()

	runtime := newConsensusDecisionTestRuntime(t)
	run, err := runtime.Spawn(context.Background(), SpawnRequest{
		Prompt: "compare approaches", Mode: "consensus",
		Consensus: &ConsensusSpec{
			Automatic: true, ExpectedQualityDelta: 0.1, DecisionReason: "hard evaluation",
			Variants: []ProviderOverride{{Alias: "one"}, {Alias: "two"}},
		},
	})
	if err != nil {
		t.Fatalf("spawn invalid automatic consensus: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := runtime.Wait(ctx, run.ID)
	if err != nil {
		t.Fatalf("wait invalid automatic consensus: %v", err)
	}
	if final.Status != RunStatusFailed || !strings.Contains(final.Error, "OH-001 baseline") {
		t.Fatalf("automatic consensus without baseline = %+v", final)
	}
}

func TestAutomaticConsensusRecordsDecisionBudgetAndObservedResult(t *testing.T) {
	t.Parallel()

	runtime := newConsensusDecisionTestRuntime(t)
	run, err := runtime.Spawn(context.Background(), SpawnRequest{
		Prompt: "compare approaches", Mode: "consensus",
		Consensus: &ConsensusSpec{
			Automatic: true, BaselineID: "OH-001:parallel_fanout:v1",
			ExpectedQualityDelta: 0.15, DecisionReason: "baseline predicts higher verification recall",
			Variants: []ProviderOverride{{Alias: "one"}, {Alias: "two"}},
		},
	})
	if err != nil {
		t.Fatalf("spawn automatic consensus: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := runtime.Wait(ctx, run.ID)
	if err != nil {
		t.Fatalf("wait automatic consensus: %v", err)
	}
	decision := final.ConsensusDecision
	if final.Status != RunStatusCompleted || decision == nil {
		t.Fatalf("automatic consensus final = %+v", final)
	}
	if !decision.Automatic || decision.BaselineID != "OH-001:parallel_fanout:v1" || decision.ExpectedQualityDelta != 0.15 || decision.Fanout != 2 {
		t.Fatalf("automatic consensus decision = %+v", decision)
	}
	if decision.ExpectedTokens <= 0 || decision.TokenBudget <= 0 || decision.CostBudgetUSD <= 0 || decision.ObservedCompleted != 2 || decision.ObservedFailed != 0 || decision.ObservedTokens <= 0 || decision.ObservedOutcome != "completed" || decision.ObservedAt == "" {
		t.Fatalf("automatic consensus observed result = %+v", decision)
	}
}

func newConsensusDecisionTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime := NewRuntime(RuntimeOptions{
		Enabled: true, SessionStore: session.NewStore(t.TempDir()), AgentRuntimeConsensusEnabled: true,
		AgentRuntimeConsensusMaxFanout: 3, AgentRuntimeConsensusBudgetTokens: 20000,
		AgentRuntimeConsensusBudgetUSD: 0.5, AgentRuntimeConsensusTimeoutSeconds: 5,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			if strings.Contains(prompt, "Candidate answers") {
				return "synthesized", nil
			}
			return "candidate", nil
		},
		ResolveProviderOverride: func(tier string, override *ProviderOverride) (ResolvedProviderOverride, error) {
			return ResolvedProviderOverride{Alias: override.Alias, Kind: "test", Model: "test-model", Tier: tier}, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, runtime) })
	return runtime
}
