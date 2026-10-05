import test from 'node:test'
import assert from 'node:assert/strict'
import { hasAgentRuntimeUsage, normalizeAgentRuntimeViewMode } from '../src/lib/agentruntime-view.ts'
import type { AgentRuntimeRun, ConsensusVariantRecord } from '../src/lib/types.ts'

const run: AgentRuntimeRun = { run_id: 'run', agent: 'worker', status: 'completed' }
const variant: ConsensusVariantRecord = { variant_idx: 0 }

test('run view keeps supported modes and falls back to list for legacy flow or unknown values', () => {
  for (const mode of ['list', 'tree', 'gantt']) assert.equal(normalizeAgentRuntimeViewMode(mode), mode)
  for (const mode of ['flow', '', 'unknown', null, undefined]) assert.equal(normalizeAgentRuntimeViewMode(mode), 'list')
})

test('cost card is hidden without recorded usage, including budget-only runs', () => {
  for (const value of [null, undefined, run,
    { ...run, consensus_budget_usd: 10 },
    { ...run, consensus_cost_usd: 0, consensus_variants: [] },
    { ...run, consensus_variants: [variant] },
    { ...run, consensus_variants: [{ ...variant, cost_usd: 0, tokens_in: 0, tokens_out: 0 }] },
  ]) assert.equal(hasAgentRuntimeUsage(value), false)
})

test('cost card shows recorded run cost, variant cost, or either token count independently', () => {
  assert.equal(hasAgentRuntimeUsage({ ...run, consensus_cost_usd: 0.01 }), true)
  for (const usage of [{ cost_usd: 0.01 }, { tokens_in: 1 }, { tokens_out: 1 }]) {
    assert.equal(hasAgentRuntimeUsage({
      ...run,
      consensus_cost_usd: 0,
      consensus_variants: [variant, { ...variant, variant_idx: 1, ...usage }],
    }), true)
  }
})
