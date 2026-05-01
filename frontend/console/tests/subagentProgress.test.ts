import test from 'node:test'
import assert from 'node:assert/strict'

import {
  agentRuntimeRunHref,
  buildSubagentProgress,
  shortRunID,
} from '../src/lib/subagentProgress.ts'

test('subagent progress builds a live running card from compact tool args', () => {
  const progress = buildSubagentProgress({
    toolName: 'subagents_run',
    toolArgs: JSON.stringify({
      agent: 'explorer',
      mode: 'parallel',
      count: 2,
      tasks: [
        { title: 'Check API', tier: 'light' },
        { title: 'Review UI' },
      ],
    }),
    toolDone: false,
  })

  assert.ok(progress)
  assert.equal(progress.complete, false)
  assert.equal(progress.agent, 'explorer')
  assert.equal(progress.count, 2)
  assert.equal(progress.running, 2)
  assert.equal(progress.completed, 0)
  assert.equal(progress.failed, 0)
  assert.deepEqual(progress.tasks.map((task) => task.title), ['Check API', 'Review UI'])
  assert.deepEqual(progress.tasks.map((task) => task.status), ['running', 'running'])
})

test('subagent progress builds completed counts and run links from compact results', () => {
  const progress = buildSubagentProgress({
    toolName: 'subagents_run',
    toolArgs: JSON.stringify({
      agent: 'explorer',
      tasks: [{ title: 'Check API' }, { title: 'Review UI' }],
    }),
    toolResult: JSON.stringify({
      count: 2,
      agent: 'explorer',
      subagents: [
        {
          run_id: 'run_1234567890abcdef',
          session_id: 'session_a',
          title: 'Check API',
          status: 'completed',
          tier: 'light',
          summary: 'API looks good.',
        },
        {
          run_id: 'run_failed',
          title: 'Review UI',
          status: 'failed',
          error: 'model failed',
        },
      ],
    }),
    toolDone: true,
  })

  assert.ok(progress)
  assert.equal(progress.complete, true)
  assert.equal(progress.completed, 1)
  assert.equal(progress.failed, 1)
  assert.equal(progress.running, 0)
  assert.equal(progress.tasks[0].runId, 'run_1234567890abcdef')
  assert.equal(progress.tasks[0].href, '/console/agentruntime/runs/run_1234567890abcdef')
  assert.equal(progress.tasks[1].error, 'model failed')
  assert.equal(agentRuntimeRunHref('run_123'), '/console/agentruntime/runs/run_123')
  assert.equal(shortRunID('run_1234567890abcdef'), 'run_123456...')
})

test('subagent progress ignores non-subagent tools and malformed previews', () => {
  assert.equal(buildSubagentProgress({ toolName: 'read_file', toolDone: false }), null)
  assert.equal(buildSubagentProgress({ toolName: 'subagents_run', toolArgs: '{"tasks":' }), null)
})
