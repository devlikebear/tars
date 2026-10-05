import type { AgentRuntimeRun } from './types'

export type AgentRuntimeViewMode = 'list' | 'tree' | 'gantt'

// The view is currently local state. Unknown/legacy modes must use the list.
export function normalizeAgentRuntimeViewMode(value: unknown): AgentRuntimeViewMode {
  return value === 'tree' || value === 'gantt' ? value : 'list'
}

// Match the recorded cost/token fields read by AgentRuntimeCostFlow, not budgets.
export function hasAgentRuntimeUsage(run: AgentRuntimeRun | null | undefined): boolean {
  if (!run) return false
  return (run.consensus_cost_usd ?? 0) > 0 || (run.consensus_variants ?? []).some((variant) =>
    (variant.cost_usd ?? 0) > 0 || (variant.tokens_in ?? 0) + (variant.tokens_out ?? 0) > 0,
  )
}
