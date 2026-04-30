import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildAgentRuntimeFlowGraph,
  buildAgentRuntimeGanttRows,
  buildAgentRuntimeTreeRows,
} from '../src/lib/agentruntime-graph.ts'
import type { AgentRuntimeRun } from '../src/lib/types.ts'

const runs: AgentRuntimeRun[] = [
  {
    run_id: 'run_parent',
    agent: 'planner',
    status: 'completed',
    tier: 'heavy',
    created_at: '2026-05-01T00:00:00Z',
    started_at: '2026-05-01T00:00:00Z',
    completed_at: '2026-05-01T00:02:00Z',
  },
  {
    run_id: 'run_child_b',
    parent_run_id: 'run_parent',
    agent: 'backend',
    status: 'failed',
    tier: 'standard',
    depth: 1,
    created_at: '2026-05-01T00:00:20Z',
    started_at: '2026-05-01T00:00:25Z',
    completed_at: '2026-05-01T00:01:00Z',
  },
  {
    run_id: 'run_child_a',
    parent_run_id: 'run_parent',
    agent: 'frontend',
    status: 'running',
    tier: 'light',
    depth: 1,
    created_at: '2026-05-01T00:00:10Z',
    started_at: '2026-05-01T00:00:15Z',
    updated_at: '2026-05-01T00:01:40Z',
    consensus_variants: [
      { variant_idx: 0, alias: 'fast', status: 'completed', started_at: '2026-05-01T00:00:20Z', finished_at: '2026-05-01T00:00:50Z', tokens_in: 10, tokens_out: 20 },
      { variant_idx: 1, alias: 'deep', status: 'running', started_at: '2026-05-01T00:00:30Z', finished_at: '2026-05-01T00:01:30Z', tokens_in: 30, tokens_out: 40 },
    ],
  },
]

test('buildAgentRuntimeTreeRows orders parents before timestamp-sorted children', () => {
  const rows = buildAgentRuntimeTreeRows(runs)

  assert.deepEqual(rows.map((row) => row.runId), ['run_parent', 'run_child_a', 'run_child_b'])
  assert.deepEqual(rows.map((row) => row.depth), [0, 1, 1])
  assert.equal(rows[0].tierShape, 'heavy')
  assert.equal(rows[1].statusKind, 'running')
  assert.equal(rows[2].statusKind, 'error')
})

test('buildAgentRuntimeGanttRows scales runs and consensus variants onto one timeline', () => {
  const result = buildAgentRuntimeGanttRows(runs)

  assert.equal(result.hasTimeline, true)
  assert.equal(result.rows.length, 3)
  assert.equal(result.rows[0].runId, 'run_parent')
  assert.equal(result.rows[1].runId, 'run_child_a')
  assert.equal(result.rows[1].variants.length, 2)
  assert.equal(result.rows[1].variants[0].label, 'fast')
  assert.equal(result.rows[1].variants[1].tokens, 70)
  assert.ok(result.rows[1].leftPercent > result.rows[0].leftPercent)
  assert.ok(result.rows[1].widthPercent > 0)
})

test('buildAgentRuntimeFlowGraph projects runs into tier-shaped nodes and spawn/variant edges', () => {
  const graph = buildAgentRuntimeFlowGraph(runs)

  assert.equal(graph.nodes.length, 5)
  assert.equal(graph.edges.length, 4)
  assert.equal(graph.nodes.find((node) => node.id === 'run_parent')?.data.tierShape, 'heavy')
  assert.equal(graph.nodes.find((node) => node.id === 'run_child_a')?.data.tokens, 100)
  assert.ok(graph.edges.some((edge) => edge.id === 'spawn-run_parent-run_child_a' && edge.animated))
  assert.ok(graph.edges.some((edge) => edge.id === 'variant-run_child_a-1' && edge.data?.kind === 'variant'))
})

test('buildAgentRuntimeFlowGraph filters by tier, status, and session', () => {
  const graph = buildAgentRuntimeFlowGraph([
    { ...runs[0], session_id: 's1' },
    { ...runs[1], session_id: 's2' },
    { ...runs[2], session_id: 's1' },
  ], {
    tier: 'light',
    status: 'running',
    session: 's1',
  })

  assert.deepEqual(graph.nodes.map((node) => node.id), ['run_child_a', 'run_child_a-variant-0', 'run_child_a-variant-1'])
  assert.deepEqual(graph.edges.map((edge) => edge.data?.kind), ['variant', 'variant'])
})
