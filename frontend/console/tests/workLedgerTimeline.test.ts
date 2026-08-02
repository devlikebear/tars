import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  buildWorkLedgerTimeline,
  latestWorkLedgerSequence,
  resumableWorkLedgerSteps,
  workLedgerCanCancel,
} from '../src/lib/workLedger.ts'
import { cancelWorkLedger, getSessionWorkLedger, resumeWorkLedgerStep } from '../src/lib/api.ts'
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
    schedules: [],
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

test('Tasks panel loads and controls a session-scoped durable work ledger timeline', () => {
  assert.match(apiSource, /\['session', 'legacy-session'\]/)
  assert.match(apiSource, /source_id/)
  assert.match(apiSource, /\/v1\/work\/works/)
  assert.match(apiSource, /\/timeline/)
  assert.match(apiSource, /cancelWorkLedger/)
  assert.match(apiSource, /resumeWorkLedgerStep/)
  assert.match(apiSource, /watchWorkLedger/)
  assert.match(tasksPanelSource, /type TabId = 'tasks' \| 'contract' \| 'evidence' \| 'timeline'/)
  assert.match(tasksPanelSource, /aria-label="Durable proof records"/)
  assert.match(tasksPanelSource, /proof\.verifier_id/)
  assert.match(tasksPanelSource, /proof\.subject_digest/)
  assert.match(tasksPanelSource, /Durable work ledger timeline/)
  assert.match(tasksPanelSource, /buildWorkLedgerTimeline/)
  assert.match(tasksPanelSource, /Cancel work/)
  assert.match(tasksPanelSource, /Resume step/)
})

test('durable work controls expose only safe operator actions', () => {
  const projection = {
    work: {
      schema_version: 1,
      id: 'work-review',
      workspace_id: 'default',
      kind: 'subagent_flow',
      source: 'session',
      source_id: 'session-42',
      idempotency_key: 'subagent-flow:flow-1',
      title: 'Review durable flow',
      contract: {},
      metadata: {},
      state: 'review',
      priority: 0,
      actor_id: 'scheduler',
      version: 8,
      created_at: '2026-08-02T00:00:00Z',
      updated_at: '2026-08-02T00:08:00Z',
    },
    steps: [
      {
        schema_version: 1,
        id: 'step-review',
        workspace_id: 'default',
        work_id: 'work-review',
        idempotency_key: 'review',
        title: 'Review me',
        state: 'review',
        position: 1,
        actor_id: 'scheduler',
        version: 4,
        created_at: '2026-08-02T00:00:00Z',
        updated_at: '2026-08-02T00:08:00Z',
      },
      {
        schema_version: 1,
        id: 'step-done',
        workspace_id: 'default',
        work_id: 'work-review',
        idempotency_key: 'done',
        title: 'Already done',
        state: 'done',
        position: 2,
        actor_id: 'scheduler',
        version: 2,
        created_at: '2026-08-02T00:00:00Z',
        updated_at: '2026-08-02T00:02:00Z',
      },
    ],
    schedules: [
      {
        schema_version: 1,
        workspace_id: 'default',
        work_id: 'work-review',
        step_id: 'step-review',
        policy: {
          max_attempts: 1,
          retry_limit: 0,
          replan_limit: 0,
          decompose_limit: 0,
          escalation_state: 'review',
        },
        attempt_count: 1,
        cycle_attempt_count: 1,
        consumed_iterations: 1,
        consumed_tokens: 100,
        consumed_cost_usd: 0.01,
        next_action: 'execute',
        last_disposition: 'review',
        blocked_reason: 'operator decision required',
        human_resume_required: true,
        updated_at: '2026-08-02T00:08:00Z',
      },
    ],
    dependencies: [],
    attempts: [],
    events: [
      { schema_version: 1, sequence: 7, id: 'event-7', workspace_id: 'default', work_id: 'work-review', type: 'step.review_requested', actor_id: 'scheduler', payload: {}, created_at: '2026-08-02T00:07:00Z' },
      { schema_version: 1, sequence: 11, id: 'event-11', workspace_id: 'default', work_id: 'work-review', type: 'work.transitioned', actor_id: 'scheduler', payload: {}, created_at: '2026-08-02T00:08:00Z' },
    ],
    proofs: [],
    artifacts: [],
    approvals: [],
  } satisfies WorkLedgerProjection

  assert.equal(workLedgerCanCancel(projection), true)
  assert.deepEqual(resumableWorkLedgerSteps(projection).map((step) => step.id), ['step-review'])
  assert.equal(latestWorkLedgerSequence(projection), 11)

  projection.work.state = 'done'
  assert.equal(workLedgerCanCancel(projection), false)
})

test('work ledger API prefers durable session work and posts operator reasons', async () => {
  const originalFetch = globalThis.fetch
  const requests: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (input, init) => {
    const url = String(input)
    requests.push({ url, init })
    if (url.includes('/v1/work/works?')) {
      const works = url.includes('source=session&') ? [] : [{ id: 'legacy-work' }]
      return new Response(JSON.stringify({ works, limit: 1, offset: 0 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    return new Response(JSON.stringify({ work: { id: 'legacy-work' }, steps: [], schedules: [], dependencies: [], attempts: [], events: [], proofs: [], artifacts: [], approvals: [] }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    const work = await getSessionWorkLedger('session-42')
    assert.equal(work?.id, 'legacy-work')
    await cancelWorkLedger('legacy-work', 'operator stopped it')
    await resumeWorkLedgerStep('legacy-work', 'step-1', 'operator approved it')
  } finally {
    globalThis.fetch = originalFetch
  }

  assert.match(requests[0].url, /source=session&source_id=session-42/)
  assert.match(requests[1].url, /source=legacy-session&source_id=session-42/)
  assert.equal(requests[2].init?.method, 'POST')
  assert.deepEqual(JSON.parse(String(requests[2].init?.body)), { reason: 'operator stopped it' })
  assert.equal(requests[3].init?.method, 'POST')
  assert.deepEqual(JSON.parse(String(requests[3].init?.body)), { reason: 'operator approved it' })
})
