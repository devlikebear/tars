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
      {
        schema_version: 1,
        sequence: 4,
        id: 'event-4',
        workspace_id: 'default',
        work_id: 'work-1',
        step_id: 'step-1',
        attempt_id: 'attempt-1',
        type: 'execution.environment_provisioned',
        actor_id: 'tars-execution-plane',
        payload: { provider: 'managed-worktree', environment_id: 'worktree:attempt-1' },
        created_at: '2026-08-02T00:03:00Z',
      },
      {
        schema_version: 1,
        sequence: 5,
        id: 'event-5',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'execution.worker_started',
        actor_id: 'tars-execution-plane',
        payload: { worker: 'native-agentruntime', provider: 'managed-worktree' },
        created_at: '2026-08-02T00:04:00Z',
      },
      {
        schema_version: 1,
        sequence: 6,
        id: 'event-6',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'execution.environment_synced',
        actor_id: 'tars-execution-plane',
        payload: { snapshot: { digest: 'sha256:abc' } },
        created_at: '2026-08-02T00:05:00Z',
      },
      {
        schema_version: 1,
        sequence: 7,
        id: 'event-7',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'execution.artifacts_collected',
        actor_id: 'tars-execution-plane',
        payload: { artifact_count: 2 },
        created_at: '2026-08-02T00:06:00Z',
      },
      {
        schema_version: 1,
        sequence: 8,
        id: 'event-8',
        workspace_id: 'default',
        work_id: 'work-1',
        type: 'execution.environment_destroyed',
        actor_id: 'tars-execution-plane',
        payload: { provider: 'managed-worktree', environment_id: 'worktree:attempt-1' },
        created_at: '2026-08-02T00:07:00Z',
      },
    ],
    proofs: [],
    artifacts: [],
    approvals: [],
  } satisfies WorkLedgerProjection

  const entries = buildWorkLedgerTimeline(projection)

  assert.deepEqual(entries.map((entry) => entry.sequence), [1, 2, 3, 4, 5, 6, 7, 8])
  assert.deepEqual(entries.map((entry) => entry.title), [
    'Work created',
    'Step created',
    'ready → running',
    'Environment provisioned',
    'Worker started',
    'Environment synchronized',
    'Artifacts collected',
    'Environment destroyed',
  ])
  assert.equal(entries[0].detail, 'Ship the durable ledger')
  assert.equal(entries[1].detail, 'Persist records')
  assert.equal(entries[2].detail, 'Approved for execution')
  assert.equal(entries[3].detail, 'managed-worktree · worktree:attempt-1')
  assert.equal(entries[4].detail, 'native-agentruntime · managed-worktree')
  assert.equal(entries[5].detail, 'sha256:abc')
  assert.equal(entries[6].detail, '2 artifacts')
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

test('work ledger timeline names reviewed capability lifecycle evidence', () => {
  const base = {
    work: {
      schema_version: 1,
      id: 'work-capability',
      workspace_id: 'default',
      kind: 'capability-improvement',
      idempotency_key: 'capability:test',
      title: 'Review capability',
      contract: {},
      metadata: {},
      state: 'review',
      priority: 0,
      actor_id: 'self-improvement',
      version: 1,
      created_at: '2026-08-02T00:00:00Z',
      updated_at: '2026-08-02T00:00:00Z',
    },
    steps: [], schedules: [], dependencies: [], attempts: [], proofs: [], artifacts: [], approvals: [],
    events: [
      { schema_version: 1, sequence: 1, id: 'e1', workspace_id: 'default', work_id: 'work-capability', type: 'capability.version_created', actor_id: 'self-improvement', payload: { capability_name: 'review-helper', version: 1, state: 'candidate' }, created_at: '2026-08-02T00:00:00Z' },
      { schema_version: 1, sequence: 2, id: 'e2', workspace_id: 'default', work_id: 'work-capability', type: 'capability.evaluation_recorded', actor_id: 'capability-evaluator', payload: { stage: 'sandbox', status: 'passed' }, created_at: '2026-08-02T00:01:00Z' },
      { schema_version: 1, sequence: 3, id: 'e3', workspace_id: 'default', work_id: 'work-capability', type: 'capability.transitioned', actor_id: 'operator', payload: { from_state: 'canary', to_state: 'promoted', reason: 'approved canary' }, created_at: '2026-08-02T00:02:00Z' },
      { schema_version: 1, sequence: 4, id: 'e4', workspace_id: 'default', work_id: 'work-capability', type: 'capability.outcome_recorded', actor_id: 'scheduler', payload: { status: 'succeeded', verifier_status: 'passed' }, created_at: '2026-08-02T00:03:00Z' },
      { schema_version: 1, sequence: 5, id: 'e5', workspace_id: 'default', work_id: 'work-capability', type: 'capability.regression_detected', actor_id: 'scheduler', payload: { status: 'failed', verifier_status: 'failed' }, created_at: '2026-08-02T00:04:00Z' },
    ],
  } satisfies WorkLedgerProjection

  const entries = buildWorkLedgerTimeline(base)
  assert.deepEqual(entries.map((entry) => entry.title), [
    'Capability version created',
    'Capability evaluation recorded',
    'canary → promoted',
    'Capability outcome recorded',
    'Capability regression needs review',
  ])
  assert.equal(entries[1].detail, 'sandbox · passed')
  assert.equal(entries[3].detail, 'succeeded · passed')
  assert.equal(entries[4].detail, 'failed · failed')
})

test('work ledger timeline explains remote worker and A2A lifecycle evidence', () => {
  const projection = {
    work: {
      schema_version: 1, id: 'work-remote', workspace_id: 'default', kind: 'remote',
      idempotency_key: 'remote:test', title: 'Run remotely', contract: {}, metadata: {},
      state: 'running', priority: 0, actor_id: 'scheduler', version: 1,
      created_at: '2026-08-02T00:00:00Z', updated_at: '2026-08-02T00:05:00Z',
    },
    steps: [], schedules: [], dependencies: [], attempts: [], proofs: [], artifacts: [], approvals: [],
    events: [
      { schema_version: 1, sequence: 1, id: 'e1', workspace_id: 'default', work_id: 'work-remote', type: 'worker.placement_created', actor_id: 'worker-control', payload: { worker_id: 'worker-a', placement_id: 'placement-a' }, created_at: '2026-08-02T00:01:00Z' },
      { schema_version: 1, sequence: 2, id: 'e2', workspace_id: 'default', work_id: 'work-remote', type: 'worker.workspace_synced', actor_id: 'worker-control', payload: { mode: 'directory', file_count: 12, total_bytes: 2048, digest: 'sha256:abc' }, created_at: '2026-08-02T00:02:00Z' },
      { schema_version: 1, sequence: 3, id: 'e3', workspace_id: 'default', work_id: 'work-remote', type: 'worker.lost', actor_id: 'worker-control', payload: { worker_id: 'worker-a', placement_id: 'placement-a' }, created_at: '2026-08-02T00:03:00Z' },
      { schema_version: 1, sequence: 4, id: 'e4', workspace_id: 'default', work_id: 'work-remote', type: 'a2a.task_submitted', actor_id: 'a2a', payload: { task_id: 'task-a', protocol_version: '1.0' }, created_at: '2026-08-02T00:04:00Z' },
      { schema_version: 1, sequence: 5, id: 'e5', workspace_id: 'default', work_id: 'work-remote', type: 'a2a.artifact_quarantined', actor_id: 'a2a', payload: { task_id: 'task-a', quarantined_parts: 2 }, created_at: '2026-08-02T00:05:00Z' },
    ],
  } satisfies WorkLedgerProjection

  const entries = buildWorkLedgerTimeline(projection)
  assert.deepEqual(entries.map((entry) => entry.title), [
    'Remote placement created',
    'Workspace synchronized',
    'Remote worker lost',
    'A2A task submitted',
    'A2A artifact quarantined',
  ])
  assert.equal(entries[0].detail, 'worker-a · placement-a')
  assert.equal(entries[1].detail, 'directory · 12 files · 2.0 KB · sha256:abc')
  assert.equal(entries[2].detail, 'worker-a · placement-a')
  assert.equal(entries[3].detail, 'task-a · protocol 1.0')
  assert.equal(entries[4].detail, 'task-a · 2 parts')
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
