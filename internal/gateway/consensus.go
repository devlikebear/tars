package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
)

func (r *Runtime) publishRunEvent(runID string, event RunEvent) {
	if r == nil || r.runEvents == nil {
		return
	}
	event.RunID = strings.TrimSpace(runID)
	if event.Timestamp == "" {
		event.Timestamp = r.nowFn().UTC().Format(time.RFC3339)
	}
	r.runEvents.Publish(event.RunID, event)
}

func (r *Runtime) SubscribeRunEvents(runID string) (<-chan RunEvent, func()) {
	if r == nil || r.runEvents == nil {
		ch := make(chan RunEvent)
		close(ch)
		return ch, func() {}
	}
	return r.runEvents.Subscribe(strings.TrimSpace(runID))
}

func (r *Runtime) runConsensus(ctx context.Context, state *runState, executor AgentExecutor) (string, error) {
	if !r.opts.GatewayConsensusEnabled {
		return "", fmt.Errorf("consensus is disabled (gateway_consensus_enabled=false)")
	}
	spec := state.req.Consensus
	if spec == nil {
		return "", fmt.Errorf("consensus config is required")
	}
	variants := sanitizeConsensusVariants(spec.Variants)
	if len(variants) == 0 {
		return "", fmt.Errorf("consensus requires at least one variant")
	}
	if len(variants) > r.opts.GatewayConsensusMaxFanout {
		return "", fmt.Errorf("consensus fanout %d exceeds gateway_consensus_max_fanout=%d", len(variants), r.opts.GatewayConsensusMaxFanout)
	}
	allowedAliases := sanitizeStringList(r.opts.GatewayConsensusAllowedAliases)
	resolved := make([]ResolvedProviderOverride, 0, len(variants))
	for _, variant := range variants {
		if err := validateConsensusAlias(variant, allowedAliases); err != nil {
			return "", err
		}
		if r.opts.ResolveProviderOverride == nil {
			return "", fmt.Errorf("provider override resolver is not configured")
		}
		resolvedVariant, err := r.opts.ResolveProviderOverride(strings.TrimSpace(state.run.Tier), &variant)
		if err != nil {
			return "", err
		}
		resolved = append(resolved, resolvedVariant)
	}

	basePromptTokens := estimateTextTokens(state.run.Prompt)
	outputBudgetPerVariant := maxConsensusInt(256, minInt(1024, basePromptTokens))
	estimatedTokens := len(resolved)*(basePromptTokens+outputBudgetPerVariant) + (basePromptTokens + outputBudgetPerVariant)
	if r.opts.GatewayConsensusBudgetTokens > 0 && estimatedTokens > r.opts.GatewayConsensusBudgetTokens {
		return "", fmt.Errorf("consensus_budget_exceeded: %d > %d", estimatedTokens, r.opts.GatewayConsensusBudgetTokens)
	}
	estimatedUSD := 0.0
	for _, variant := range resolved {
		estimatedUSD += r.estimateCostUSD(variant.Kind, variant.Model, basePromptTokens, outputBudgetPerVariant)
	}
	if len(resolved) > 0 {
		estimatedUSD += r.estimateCostUSD(resolved[0].Kind, resolved[0].Model, basePromptTokens, outputBudgetPerVariant)
	}
	if r.opts.GatewayConsensusBudgetUSD > 0 && estimatedUSD > r.opts.GatewayConsensusBudgetUSD {
		return "", fmt.Errorf("consensus_usd_budget_exceeded: %.4f > %.4f", estimatedUSD, r.opts.GatewayConsensusBudgetUSD)
	}
	state.run.ConsensusBudgetUSD = estimatedUSD
	r.publishRunEvent(state.run.ID, RunEvent{Type: "consensus_planned", RunID: state.run.ID, VariantCount: len(resolved), TokenBudget: r.opts.GatewayConsensusBudgetTokens, CostUSDEstimate: estimatedUSD})

	if err := r.consensusRuns.Acquire(ctx); err != nil {
		return "", err
	}
	defer r.consensusRuns.Release()
	consensusCtx, cancel := context.WithTimeout(ctx, time.Duration(r.opts.GatewayConsensusTimeoutSeconds)*time.Second)
	defer cancel()

	strategy := strings.TrimSpace(spec.Strategy)
	if strategy == "" {
		strategy = "synthesize"
	}
	records := make([]ConsensusVariantRecord, len(resolved))
	var tokenSum atomic.Int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for idx := range resolved {
		idx := idx
		variant := variants[idx]
		resolvedVariant := resolved[idx]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.consensusPool.Acquire(consensusCtx); err != nil {
				mu.Lock()
				records[idx] = ConsensusVariantRecord{VariantIdx: idx, Alias: resolvedVariant.Alias, Kind: resolvedVariant.Kind, Model: resolvedVariant.Model, Status: "failed", Error: err.Error()}
				mu.Unlock()
				return
			}
			defer r.consensusPool.Release()
			startedAt := r.nowFn().UTC().Format(time.RFC3339)
			r.publishRunEvent(state.run.ID, RunEvent{Type: "consensus_variant_started", RunID: state.run.ID, VariantIdx: idx, Alias: resolvedVariant.Alias, Kind: resolvedVariant.Kind, Model: resolvedVariant.Model, Timestamp: startedAt})
			resp, err := r.executeVariantPrompt(consensusCtx, state, executor, variant, resolvedVariant)
			tokensIn := basePromptTokens
			tokensOut := estimateTextTokens(resp)
			tokenTotal := tokensIn + tokensOut
			tokenSum.Add(int64(tokenTotal))
			record := ConsensusVariantRecord{VariantIdx: idx, Alias: resolvedVariant.Alias, Kind: resolvedVariant.Kind, Model: resolvedVariant.Model, TokensIn: tokensIn, TokensOut: tokensOut, CostUSD: r.estimateCostUSD(resolvedVariant.Kind, resolvedVariant.Model, tokensIn, tokensOut), StartedAt: startedAt, FinishedAt: r.nowFn().UTC().Format(time.RFC3339)}
			if err != nil {
				record.Status = "failed"
				record.Error = strings.TrimSpace(err.Error())
			} else {
				record.Status = "completed"
				record.Response = strings.TrimSpace(resp)
			}
			mu.Lock()
			records[idx] = record
			mu.Unlock()
			r.publishRunEvent(state.run.ID, RunEvent{Type: "consensus_variant_finished", RunID: state.run.ID, VariantIdx: idx, Alias: resolvedVariant.Alias, Kind: resolvedVariant.Kind, Model: resolvedVariant.Model, TokensIn: tokensIn, TokensOut: tokensOut, CostUSDActual: record.CostUSD, Error: record.Error, Timestamp: record.FinishedAt})
			if budget := r.opts.GatewayConsensusBudgetTokens; budget > 0 && int(tokenSum.Load()) > budget {
				cancel()
			}
		}()
	}
	wg.Wait()
	state.run.ConsensusVariants = records
	if err := consensusCtx.Err(); err != nil {
		if budget := r.opts.GatewayConsensusBudgetTokens; budget > 0 && int(tokenSum.Load()) > budget {
			return "", fmt.Errorf("consensus_budget_exceeded: %d > %d", tokenSum.Load(), budget)
		}
		return "", err
	}
	successes := make([]ConsensusVariantRecord, 0, len(records))
	for _, record := range records {
		state.run.ConsensusCostUSD += record.CostUSD
		if record.Status == "completed" && strings.TrimSpace(record.Response) != "" {
			successes = append(successes, record)
		}
	}
	if len(successes) == 0 {
		return "", fmt.Errorf("all consensus variants failed")
	}
	r.publishRunEvent(state.run.ID, RunEvent{Type: "consensus_aggregating", RunID: state.run.ID, Strategy: strategy})
	finalResp, err := r.aggregateConsensus(consensusCtx, state, executor, strategy, successes)
	if err != nil {
		finalResp = successes[0].Response
	}
	finalTokens := estimateTextTokens(finalResp)
	if len(successes) > 0 {
		state.run.ConsensusCostUSD += r.estimateCostUSD(successes[0].Kind, successes[0].Model, basePromptTokens, finalTokens)
	}
	r.publishRunEvent(state.run.ID, RunEvent{Type: "consensus_finished", RunID: state.run.ID, Strategy: strategy, FinalTokens: finalTokens, CostUSDActual: state.run.ConsensusCostUSD})
	return strings.TrimSpace(finalResp), nil
}

func (r *Runtime) executeVariantPrompt(ctx context.Context, state *runState, executor AgentExecutor, override ProviderOverride, resolved ResolvedProviderOverride) (string, error) {
	if executor == nil {
		return "", fmt.Errorf("agent executor is not configured")
	}
	execCtx := serverauth.WithWorkspaceID(ctx, state.run.WorkspaceID)
	execCtx = usage.WithCallMeta(execCtx, usage.CallMeta{Source: "agent_run", SessionID: state.run.SessionID, RunID: state.run.ID})
	execCtx = llm.WithSelectionMetadata(execCtx, llm.SelectionMetadata{SessionID: state.run.SessionID, RunID: state.run.ID, AgentName: state.run.Agent, FlowID: state.run.FlowID, StepID: state.run.StepID, Provider: resolved.Kind, Model: resolved.Model, Tier: llm.Tier(resolved.Tier), Source: "task"})
	return executor.Execute(execCtx, ExecuteRequest{RunID: state.run.ID, WorkspaceID: state.run.WorkspaceID, SessionID: state.run.SessionID, Prompt: state.run.Prompt, AllowedTools: resolveRunAllowedTools(r.opts.WorkspaceDir, gatewayAgentInfo(executor).ToolsAllow), Tier: resolved.Tier, ProviderOverride: &override})
}

func (r *Runtime) aggregateConsensus(ctx context.Context, state *runState, executor AgentExecutor, strategy string, successes []ConsensusVariantRecord) (string, error) {
	if strings.ToLower(strings.TrimSpace(strategy)) != "synthesize" {
		return "", fmt.Errorf("consensus strategy %q is not implemented", strategy)
	}
	b := strings.Builder{}
	b.WriteString("Synthesize the best final answer by combining correct information, resolving contradictions, and discarding errors. Preserve all specific identifiers, code, and numbers that appear in any candidate. Do not add facts that no candidate supports.\n\n")
	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(state.run.Prompt))
	b.WriteString("\n\nCandidate answers:\n")
	for i, candidate := range successes {
		b.WriteString(fmt.Sprintf("%d. [alias=%s model=%s]\n%s\n\n", i+1, candidate.Alias, candidate.Model, strings.TrimSpace(candidate.Response)))
	}
	if executor == nil {
		return "", fmt.Errorf("agent executor is not configured")
	}
	execCtx := serverauth.WithWorkspaceID(ctx, state.run.WorkspaceID)
	execCtx = usage.WithCallMeta(execCtx, usage.CallMeta{Source: "agent_run", SessionID: state.run.SessionID, RunID: state.run.ID})
	execCtx = llm.WithSelectionMetadata(execCtx, llm.SelectionMetadata{SessionID: state.run.SessionID, RunID: state.run.ID, AgentName: state.run.Agent, FlowID: state.run.FlowID, StepID: state.run.StepID, Tier: llm.Tier("light"), Source: "task"})
	return executor.Execute(execCtx, ExecuteRequest{RunID: state.run.ID, WorkspaceID: state.run.WorkspaceID, SessionID: state.run.SessionID, Prompt: b.String(), AllowedTools: resolveRunAllowedTools(r.opts.WorkspaceDir, gatewayAgentInfo(executor).ToolsAllow), Tier: "light"})
}

func sanitizeConsensusVariants(variants []ProviderOverride) []ProviderOverride {
	out := make([]ProviderOverride, 0, len(variants))
	for _, variant := range variants {
		alias := strings.TrimSpace(variant.Alias)
		model := strings.TrimSpace(variant.Model)
		if alias == "" && model == "" {
			continue
		}
		out = append(out, ProviderOverride{Alias: alias, Model: model})
	}
	return out
}

func validateConsensusAlias(variant ProviderOverride, allowed []string) error {
	alias := strings.TrimSpace(variant.Alias)
	if alias == "" {
		return fmt.Errorf("consensus variant alias is required")
	}
	if len(allowed) == 0 {
		return nil
	}
	for _, candidate := range allowed {
		if candidate == alias {
			return nil
		}
	}
	return fmt.Errorf("consensus_alias_not_allowed: %s", alias)
}

func (r *Runtime) estimateCostUSD(provider, model string, inputTokens, outputTokens int) float64 {
	if r == nil || r.opts.EstimateTokensCost == nil {
		return 0
	}
	value, _ := r.opts.EstimateTokensCost(provider, model, inputTokens, outputTokens)
	return value
}

func estimateTextTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return maxConsensusInt(1, len(trimmed)/4)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxConsensusInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
