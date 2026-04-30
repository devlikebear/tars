import test from 'node:test'
import assert from 'node:assert/strict'

import {
  deriveAgentRuntimeReplayBounds,
  deriveAgentRuntimeReplayState,
} from '../src/lib/agentruntime-graph.ts'
import type { AgentRuntimeRunEvent } from '../src/lib/types.ts'

const events: AgentRuntimeRunEvent[] = [
  { type: 'run_started', run_id: 'run_1', status: 'running', timestamp: '2026-05-01T00:00:00Z' },
  { type: 'tool.call', run_id: 'run_1', path: 'README.md', action: 'read', timestamp: '2026-05-01T00:00:05Z' },
  { type: 'run_finished', run_id: 'run_1', status: 'completed', timestamp: '2026-05-01T00:00:10Z' },
]

test('deriveAgentRuntimeReplayBounds returns first and last event times', () => {
  const bounds = deriveAgentRuntimeReplayBounds(events)

  assert.equal(bounds.startMs, Date.parse('2026-05-01T00:00:00Z'))
  assert.equal(bounds.endMs, Date.parse('2026-05-01T00:00:10Z'))
  assert.equal(bounds.hasTimeline, true)
})

test('deriveAgentRuntimeReplayState applies only events up to the cursor', () => {
  const state = deriveAgentRuntimeReplayState(events, Date.parse('2026-05-01T00:00:05Z'))

  assert.equal(state.appliedCount, 2)
  assert.equal(state.status, 'running')
  assert.equal(state.lastEventType, 'tool.call')
  assert.deepEqual(state.filePaths, ['README.md'])
})
