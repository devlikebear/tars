import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildWorkLedgerTimeline } from '../src/lib/workLedger.ts'
import type { WorkLedgerProjection } from '../src/lib/types.ts'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')

test('work ledger timeline orders durable events and exposes operator labels', () => {
  const projection = {
    work: {
      schema_version: 1,
      id: 'work-1',
      workspace_id: 'default',
      kind: 'session',
      source: 'legacy-session',
      source_id: 'session-42',
      idempotency_key: 'import:session-42',
      title: 'Ship the durable ledger',
      contract: {},
      metadata: {},
      state: 'running',
      priority: 0,
      actor_id: 'importer',
      version: 2,
      created_at: '2026-08-02T00:00:00Z',
      updated_at: '2026-08-02T00:02:00Z',
    },
    steps: [],
    dependencies: [],
    attempts: [],
    events: [
      {
        schema_version: 1,
        sequence: 3,
        id: 'event-3',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'work.transitioned',
        from_state: 'ready',
        to_state: 'running',
        actor_id: 'operator',
        payload: { reason: 'Approved for execution' },
        created_at: '2026-08-02T00:02:00Z',
      },
      {
        schema_version: 1,
        sequence: 1,
        id: 'event-1',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'work.created',
        actor_id: 'importer',
        payload: { source: 'legacy-session' },
        created_at: '2026-08-02T00:00:00Z',
      },
      {
        schema_version: 1,
        sequence: 2,
        id: 'event-2',
        workspace_id: 'default',
        work_id: 'work-1',
        step_id: 'step-1',
        type: 'step.created',
        actor_id: 'importer',
        payload: { title: 'Persist records', position: 1 },
        created_at: '2026-08-02T00:01:00Z',
      },
    ],
    proofs: [],
    artifacts: [],
    approvals: [],
  } satisfies WorkLedgerProjection

  const entries = buildWorkLedgerTimeline(projection)

  assert.deepEqual(entries.map((entry) => entry.sequence), [1, 2, 3])
  assert.deepEqual(entries.map((entry) => entry.title), [
    'Work created',
    'Step created',
    'ready → running',
  ])
  assert.equal(entries[0].detail, 'Ship the durable ledger')
  assert.equal(entries[1].detail, 'Persist records')
  assert.equal(entries[2].detail, 'Approved for execution')
})

test('Tasks panel loads a session-scoped, read-only work ledger timeline', () => {
  assert.match(apiSource, /source.*legacy-session/)
  assert.match(apiSource, /source_id/)
  assert.match(apiSource, /\/v1\/work\/works/)
  assert.match(apiSource, /\/timeline/)
  assert.match(tasksPanelSource, /type TabId = 'tasks' \| 'contract' \| 'evidence' \| 'timeline'/)
  assert.match(tasksPanelSource, /Read-only work ledger timeline/)
  assert.match(tasksPanelSource, /buildWorkLedgerTimeline/)
})
