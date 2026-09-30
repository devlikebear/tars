import test from 'node:test'
import assert from 'node:assert/strict'

import {
  activityChanges,
  activityMap,
  boardCounts,
  boardStatus,
  groupBoard,
  needsInput,
  needsInputHint,
  repoName,
  type BoardSession,
} from '../src/lib/sessionBoard.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as ActivityModule from '../src/lib/stores/sessionActivity.svelte.ts'

const { SessionActivityStore } = await compileSvelteModule<typeof ActivityModule>('src/lib/stores/sessionActivity.svelte.ts')

function session(over: Partial<BoardSession>): BoardSession {
  return {
    id: 's',
    title: 'Session',
    status: 'idle',
    pending_approvals: 0,
    updated_at: '2026-09-29T10:00:00Z',
    cost_usd: 0,
    ...over,
  }
}

const t = (iso: string) => Date.parse(iso)

test('boardStatus: server status first, then finished turns nobody has seen', () => {
  const seen = { '': t('2026-09-29T09:00:00Z'), b: t('2026-09-29T12:00:00Z') }
  assert.equal(boardStatus(session({ status: 'needs_input' }), seen), 'needs_input')
  assert.equal(boardStatus(session({ status: 'running' }), seen), 'running')
  assert.equal(boardStatus(session({ id: 'a', last_turn_at: '2026-09-29T11:00:00Z' }), seen), 'done_unread')
  assert.equal(boardStatus(session({ id: 'b', last_turn_at: '2026-09-29T11:00:00Z' }), seen), 'idle')
  assert.equal(boardStatus(session({ id: 'c', last_turn_at: '2026-09-29T08:00:00Z' }), seen), 'idle', 'before the baseline')
  assert.equal(boardStatus(session({ id: 'd' }), seen), 'idle', 'no turn yet')
  assert.equal(boardStatus(session({ id: 'e', last_turn_at: '2026-09-29T11:00:00Z' }), {}), 'idle', 'nothing tracked')
})

test('groupBoard groups by repository, urgent groups first, no-folder group last', () => {
  const sessions = [
    session({ id: 'n1', title: 'Notes', updated_at: '2026-09-29T12:00:00Z' }),
    session({ id: 'a1', title: 'API fix', repo: '/src/api', updated_at: '2026-09-29T11:00:00Z' }),
    session({ id: 'w1', title: 'Web', repo: '/src/web/', updated_at: '2026-09-29T09:00:00Z', status: 'needs_input', pending_approvals: 1 }),
    session({ id: 'a2', title: 'API docs', repo: '/src/api', updated_at: '2026-09-29T10:00:00Z', pinned_at: '2026-09-01T00:00:00Z' }),
  ]
  const groups = groupBoard(sessions, {})
  assert.deepEqual(groups.map((g) => g.key), ['/src/web/', '/src/api', ''])
  assert.equal(groups[0].name, 'web')
  assert.equal(groups[0].counts.needs_input, 1)
  assert.deepEqual(groups[1].cards.map((c) => c.id), ['a2', 'a1'], 'pinned leads')
  assert.equal(groups[2].name, '')

  assert.deepEqual(groupBoard(sessions, {}, { filter: 'needs_input' }).map((g) => g.key), ['/src/web/'])
  assert.deepEqual(groupBoard(sessions, {}, { query: 'DOCS' }).flatMap((g) => g.cards.map((c) => c.id)), ['a2'])
  assert.deepEqual(groupBoard(sessions, {}, { query: 'web' }).flatMap((g) => g.cards.map((c) => c.id)), ['w1'], 'matches the repo path')
})

test('groupBoard sorts cards by status, title, or cost', () => {
  const sessions = [
    session({ id: 'x', title: 'Beta', repo: '/r', cost_usd: 1, status: 'running' }),
    session({ id: 'y', title: 'Alpha', repo: '/r', cost_usd: 3 }),
    session({ id: 'z', title: 'Gamma', repo: '/r', cost_usd: 2, status: 'needs_input' }),
  ]
  const ids = (sort: 'status' | 'title' | 'cost') => groupBoard(sessions, {}, { sort })[0].cards.map((c) => c.id)
  assert.deepEqual(ids('status'), ['z', 'x', 'y'])
  assert.deepEqual(ids('title'), ['y', 'x', 'z'])
  assert.deepEqual(ids('cost'), ['y', 'z', 'x'])
})

test('boardCounts counts every session before filtering', () => {
  const counts = boardCounts([session({ id: 'a', status: 'running' }), session({ id: 'b' }), session({ id: 'c', status: 'needs_input' })], {})
  assert.deepEqual(counts, { needs_input: 1, running: 1, done_unread: 0, idle: 1 })
})

test('repoName takes the last path element on any platform', () => {
  assert.equal(repoName('/home/me/tars'), 'tars')
  assert.equal(repoName('C:\\work\\tars\\'), 'tars')
  assert.equal(repoName('tars'), 'tars')
})

test('activityChanges reports new questions and finished turns only', () => {
  const idle = activityMap({ running: [], pending_approvals: [] })
  const running = activityMap({ running: [{ session_id: 's1', session_title: 'Build' }], pending_approvals: [] })
  const asking = activityMap({
    running: [{ session_id: 's1', session_title: 'Build' }],
    pending_approvals: [{ request_id: 'r1', session_id: 's1' }, { request_id: 'r2', session_id: 's2', session_title: 'Docs' }],
  })
  assert.deepEqual(asking.s1, { running: true, pending: 1, queued: 0, title: 'Build' })
  assert.deepEqual(asking.s2, { running: true, pending: 1, queued: 0, title: 'Docs' }, 'a question implies a running turn')
  assert.deepEqual(activityChanges(idle, running), [])
  assert.deepEqual(activityChanges(running, asking), [
    { sessionId: 's1', title: 'Build', kind: 'needs_input' },
    { sessionId: 's2', title: 'Docs', kind: 'needs_input' },
  ])
  assert.deepEqual(activityChanges(asking, asking), [], 'still waiting is not news')
  assert.deepEqual(activityChanges(asking, running), [{ sessionId: 's2', title: 'Docs', kind: 'done' }])
  assert.deepEqual(activityChanges(running, idle), [{ sessionId: 's1', title: 'Build', kind: 'done' }])
})

// #1033: an unattended run's tool call waiting in the ops queue makes its
// session need input, without making it a running chat turn.
test('activityMap counts unattended approvals as needs input', () => {
  const queued = activityMap({
    running: [{ session_id: 's1', session_title: 'Build' }],
    pending_approvals: [],
    queued_approvals: [
      { approval_id: 'a1', session_id: 's1', source: 'cron', tool_name: 'exec' },
      { approval_id: 'a2', session_id: 's3', session_title: 'Nightly', source: 'cron', tool_name: 'exec' },
      { approval_id: 'a3', session_id: 's3', source: 'subagent', tool_name: 'write_file' },
    ],
  })
  assert.deepEqual(queued.s1, { running: true, pending: 0, queued: 1, title: 'Build' })
  assert.deepEqual(queued.s3, { running: false, pending: 0, queued: 2, title: 'Nightly' }, 'an unattended run is not a chat turn')
  assert.equal(needsInput(queued.s1), true)
  assert.equal(needsInput(queued.s3), true)
  assert.equal(needsInput(undefined), false)
  assert.equal(needsInput({ running: true, pending: 0, queued: 0, title: '' }), false)

  const text = { pending: (n: number) => `${n} chat`, queued: (n: number) => `${n} unattended` }
  assert.equal(needsInputHint(queued.s3, text), '2 unattended')
  assert.equal(needsInputHint({ running: true, pending: 1, queued: 2, title: '' }, text), '1 chat · 2 unattended')
  assert.equal(needsInputHint(undefined, text), '')

  // A server before #1033 sends no queued_approvals.
  assert.deepEqual(activityMap({ running: [], pending_approvals: [] }), {})

  const idle = activityMap({ running: [], pending_approvals: [] })
  const running = activityMap({ running: [{ session_id: 's1', session_title: 'Build' }], pending_approvals: [] })
  assert.deepEqual(activityChanges(idle, queued), [
    { sessionId: 's1', title: 'Build', kind: 'needs_input' },
    { sessionId: 's3', title: 'Nightly', kind: 'needs_input' },
  ])
  assert.deepEqual(activityChanges(running, queued), [
    { sessionId: 's1', title: 'Build', kind: 'needs_input' },
    { sessionId: 's3', title: 'Nightly', kind: 'needs_input' },
  ], 'a running session that starts waiting in the queue is news')
  assert.deepEqual(activityChanges(queued, queued), [], 'still waiting is not news')
  const chatAsking = activityMap({ running: [{ session_id: 's3' }], pending_approvals: [{ request_id: 'r1', session_id: 's3' }] })
  assert.deepEqual(activityChanges(queued, chatAsking), [{ sessionId: 's1', title: 'Build', kind: 'done' }], 'from one kind of question to the other is not news')
  assert.deepEqual(activityChanges(queued, idle), [{ sessionId: 's1', title: 'Build', kind: 'done' }], 'a reviewed unattended approval is not a finished turn')
})

type Shown = { title: string; body: string; tag: string; click: () => void }

function harness(snapshots: unknown[]) {
  const store: Record<string, string> = {}
  let now = 1_000
  let visible = true
  let permission: NotificationPermission = 'default'
  const shown: Shown[] = []
  const queue = [...snapshots]
  const activity = new SessionActivityStore(
    { getChatActivity: async () => (queue.length > 1 ? queue.shift() : queue[0]) as never },
    {
      now: () => now,
      storage: { getItem: (k) => store[k] ?? null, setItem: (k, v) => void (store[k] = v) },
      notifications: {
        permission: () => permission,
        request: async () => (permission = 'granted'),
        show: (title, body, tag, click) => shown.push({ title, body, tag, click }),
      },
      visible: () => visible,
    },
  )
  activity.text = {
    needsInput: (s) => `${s} needs input`,
    done: (s) => `${s} finished`,
    body: { needsInput: 'A tool call waits', done: 'The turn ended' },
  }
  return {
    activity,
    store,
    shown,
    setNow: (n: number) => (now = n),
    setVisible: (v: boolean) => (visible = v),
    setPermission: (p: NotificationPermission) => (permission = p),
  }
}

const quiet = { running: [], pending_approvals: [] }
const runningS1 = { running: [{ session_id: 's1', session_title: 'Build' }], pending_approvals: [] }
const askingS1 = { running: [{ session_id: 's1', session_title: 'Build' }], pending_approvals: [{ request_id: 'r1', session_id: 's1' }] }

test('the store remembers a baseline and what was seen', () => {
  const h = harness([quiet])
  assert.equal(h.activity.seen[''], 1_000)
  h.setNow(5_000)
  h.activity.setViewing('s1')
  assert.equal(h.activity.seen.s1, 5_000)
  assert.equal(JSON.parse(h.store['tars.sessionBoard.seen']).s1, 5_000)
  const again = new SessionActivityStore({ getChatActivity: async () => quiet }, {
    now: () => 9_000,
    storage: { getItem: (k) => h.store[k] ?? null, setItem: () => {} },
    visible: () => true,
  })
  assert.equal(again.seen[''], 1_000, 'the baseline survives a reload')
  assert.equal(again.permission, 'unsupported')
})

test('the store notifies about background sessions once notifications are on', async () => {
  const h = harness([quiet, runningS1, askingS1, askingS1, quiet])
  await h.activity.poll()
  await h.activity.poll()
  assert.equal(h.activity.running('s1'), true)
  assert.equal(await h.activity.enableNotifications(), true)
  assert.equal(h.store['tars.sessionBoard.notify'], '1')
  await h.activity.poll()
  assert.equal(h.activity.pending('s1'), 1)
  assert.deepEqual(h.shown.map((s) => s.title), ['Build needs input'])
  await h.activity.poll()
  assert.equal(h.shown.length, 1, 'no repeat while it still waits')

  let opened = ''
  h.activity.onOpenSession = (id) => (opened = id)
  h.shown[0].click()
  assert.equal(opened, 's1')

  await h.activity.poll()
  assert.deepEqual(h.shown.map((s) => s.tag), ['tars-needs_input-s1', 'tars-done-s1'])
})

test('the store marks and announces a session waiting in the ops queue', async () => {
  const queuedS2 = {
    running: [],
    pending_approvals: [],
    queued_approvals: [{ approval_id: 'a1', session_id: 's2', session_title: 'Nightly', source: 'cron', tool_name: 'exec' }],
  }
  const h = harness([quiet, queuedS2, queuedS2, quiet])
  h.setPermission('granted')
  await h.activity.enableNotifications()
  await h.activity.poll()
  assert.equal(h.activity.needsInput('s2'), false)
  await h.activity.poll()
  assert.equal(h.activity.needsInput('s2'), true)
  assert.equal(h.activity.queued('s2'), 1)
  assert.equal(h.activity.pending('s2'), 0, 'not a chat approval')
  assert.equal(h.activity.running('s2'), false)
  assert.deepEqual(h.shown.map((s) => s.tag), ['tars-needs_input-s2'])
  await h.activity.poll()
  assert.equal(h.shown.length, 1, 'no repeat while it still waits')
  await h.activity.poll()
  assert.equal(h.activity.needsInput('s2'), false)
  assert.equal(h.shown.length, 1, 'a reviewed approval is not a finished turn')
})

test('no notification for the session on screen, or with notifications off', async () => {
  const h = harness([runningS1, askingS1, quiet])
  h.setPermission('granted')
  await h.activity.enableNotifications()
  await h.activity.poll()
  h.activity.setViewing('s1')
  h.setNow(8_000)
  await h.activity.poll()
  assert.equal(h.shown.length, 0, 'the user is looking at it')
  h.activity.disableNotifications()
  h.setVisible(false)
  await h.activity.poll()
  assert.equal(h.shown.length, 0)
  assert.equal(h.store['tars.sessionBoard.notify'], '0')
})

test('a finished turn on screen counts as seen; a failed poll keeps state', async () => {
  const h = harness([runningS1, quiet])
  await h.activity.poll()
  h.activity.setViewing('s1')
  h.setNow(20_000)
  await h.activity.poll()
  assert.equal(h.activity.seen.s1, 20_000)

  const failing = new SessionActivityStore({ getChatActivity: async () => { throw new Error('down') } }, {
    now: () => 1,
    visible: () => true,
  })
  await failing.poll()
  assert.deepEqual(failing.activity, {})
  assert.equal(failing.version, 0)
  assert.equal(await failing.enableNotifications(), false, 'no Notification API')
})

test('formatAge and formatCost', async () => {
  const { formatAge, formatCost } = await import('../src/lib/sessionBoard.ts')
  const labels = { now: 'now', seconds: (n: number) => `${n}s`, minutes: (n: number) => `${n}m`, hours: (n: number) => `${n}h`, days: (n: number) => `${n}d` }
  assert.equal(formatAge(1_000, labels), 'now')
  assert.equal(formatAge(Number.NaN, labels), 'now')
  assert.equal(formatAge(45_000, labels), '45s')
  assert.equal(formatAge(12 * 60_000, labels), '12m')
  assert.equal(formatAge(3 * 3_600_000, labels), '3h')
  assert.equal(formatAge(47 * 3_600_000, labels), '47h')
  assert.equal(formatAge(72 * 3_600_000, labels), '3d')
  assert.equal(formatCost(0), '0.00')
  assert.equal(formatCost(0.004), '0.004')
  assert.equal(formatCost(1.256), '1.26')
})
