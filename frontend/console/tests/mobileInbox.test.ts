import test from 'node:test'
import assert from 'node:assert/strict'

import { APIRequestError } from '../src/lib/api/client.ts'
import { consoleURL, inboxOf, mergeInboxes, waitingOf, type DesktopInbox } from '../src/lib/mobile/inbox.ts'
import type { InboxAPI } from '../src/lib/mobile/desktopApi.ts'
import type { PairedDesktop } from '../src/lib/mobile/desktops.ts'
import type { FocusListItem } from '../src/lib/types.ts'
import type { ActivitySnapshot } from '../src/lib/sessionBoard.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as InboxStoreModule from '../src/lib/mobile/inboxStore.svelte.ts'

const { InboxStore } = await compileSvelteModule<typeof InboxStoreModule>('src/lib/mobile/inboxStore.svelte.ts')

function item(id: string, extra: Partial<FocusListItem> = {}): FocusListItem {
  return {
    session_id: id,
    title: `title ${id}`,
    goal: `goal ${id}`,
    current: 'review',
    open_gate: '',
    needs_input: 0,
    updated_at: '2026-11-14T10:00:00Z',
    ...extra,
  }
}

const quiet: ActivitySnapshot = { running: [], pending_approvals: [], queued_approvals: [] }

function desk(id: string, pipelines: FocusListItem[], activity: ActivitySnapshot = quiet): DesktopInbox {
  return { desktopID: id, desktopName: id.replace('https://', '').split('.')[0], pipelines, activity }
}

test('what a pipeline waits on: the server says, an older server is read from its gate and unseen cards', () => {
  assert.deepEqual(waitingOf(item('a', { waiting: { gate: 'triage', cards: 2 } })), { gate: 'triage', cards: 2 })
  assert.deepEqual(waitingOf(item('a', { open_gate: 'plan', needs_input: 1, waiting: undefined })), { gate: 'plan', cards: 0 })
  assert.deepEqual(waitingOf(item('a', { needs_input: 3 })), { gate: '', cards: 3 })
  // Goal mode answers by itself.
  assert.deepEqual(waitingOf(item('a', { open_gate: 'plan', needs_input: 1, goal_mode: true })), { gate: '', cards: 0 })
  // A current server always sends `waiting`: its word wins over unseen cards.
  assert.deepEqual(waitingOf(item('a', { needs_input: 2, waiting: { cards: 0 } })), { gate: '', cards: 0 })
})

test('one desktop: pipelines that wait, with tool calls under their pipeline, Q&A included', () => {
  const entries = inboxOf(
    desk('https://mac.ts.net', [
      item('p1', { waiting: { gate: 'plan', cards: 0 }, current: 'plan' }),
      item('p2', { qa_session_id: 'qa2' }),
      item('p3'),
    ], {
      running: [],
      pending_approvals: [{ request_id: 'r1', session_id: 'p2', tool_name: 'Bash', preview: 'npm test', asked_at: '2026-11-14T11:00:00Z' }],
      queued_approvals: [
        { approval_id: 'a1', session_id: 'qa2', tool_name: 'Read', requested_at: '2026-11-14T10:30:00Z' },
        { approval_id: 'a2', session_id: 'gone', session_title: 'Old task', tool_name: 'Write' },
      ],
    }),
  )
  assert.deepEqual(entries.map((e) => [e.sessionID, e.title, e.gate, e.approvals.map((a) => `${a.kind}:${a.id}`)]), [
    ['p1', 'title p1', 'plan', []],
    ['p2', 'title p2', '', ['queued:a1', 'permission:r1']],
    // A call in a session the list no longer names still shows.
    ['gone', 'Old task', '', ['queued:a2']],
  ])
  const p2 = entries[1]
  assert.equal(p2.key, 'https://mac.ts.net\np2')
  assert.equal(p2.updatedAt, '2026-11-14T11:00:00Z')
  assert.equal(p2.approvals[1].preview, 'npm test')
  assert.equal(p2.approvals[1].sessionID, 'p2')
  assert.equal(p2.approvals[0].sessionID, 'qa2')
})

test('every desktop in one list: tool calls first, then the latest change', () => {
  const merged = mergeInboxes([
    desk('https://a.ts.net', [
      item('old', { waiting: { cards: 1 }, updated_at: '2026-11-14T08:00:00Z' }),
      item('new', { waiting: { gate: 'merge', cards: 0 }, updated_at: '2026-11-14T12:00:00+09:00' }),
    ]),
    desk('https://b.ts.net', [item('calls', { updated_at: '2026-11-13T00:00:00Z' })], {
      running: [],
      pending_approvals: [],
      queued_approvals: [{ approval_id: 'q', session_id: 'calls' }],
    }),
  ])
  // 12:00+09:00 is 03:00Z: older than 08:00Z.
  assert.deepEqual(merged.map((e) => `${e.desktopName}/${e.sessionID}`), ['b/calls', 'a/old', 'a/new'])
})

test('a pipeline opens in its desktop console', () => {
  assert.equal(consoleURL('https://mac.ts.net/', 'sess 1'), 'https://mac.ts.net/console/focus/sess%201')
})

function paired(id: string): PairedDesktop {
  return { id, name: id.replace('https://', '').split('.')[0], baseURL: id, token: `tok-${id}`, deviceID: 'd', pairedAt: '2026-11-14T00:00:00Z' }
}

function fakeInboxAPI(lists: Record<string, FocusListItem[] | Error>, activity: Record<string, ActivitySnapshot> = {}) {
  const calls: string[] = []
  const api: InboxAPI = {
    async pipelines(target) {
      const list = lists[target.baseURL]
      if (list instanceof Error) throw list
      return list ?? []
    },
    async activity(target) {
      return activity[target.baseURL] ?? quiet
    },
    async reviewApproval(target, id, action) {
      calls.push(`${target.baseURL} ${target.token} review ${id} ${action}`)
    },
    async answerPermission(target, id, sessionID, decision) {
      calls.push(`${target.baseURL} permission ${id} ${sessionID} ${decision}`)
    },
  }
  return { api, calls }
}

test('the inbox reads every desktop that answers, and says which did not', async () => {
  const a = paired('https://a.ts.net')
  const b = paired('https://b.ts.net')
  const lists: Record<string, FocusListItem[] | Error> = {
    [a.id]: [item('p1', { waiting: { gate: 'plan', cards: 0 } })],
    [b.id]: new TypeError('Load failed'),
  }
  const { api } = fakeInboxAPI(lists)
  const inbox = new InboxStore({ api })
  await inbox.refresh([a, b])
  assert.equal(inbox.loaded, true)
  assert.deepEqual(inbox.entries.map((e) => e.sessionID), ['p1'])
  assert.deepEqual(inbox.failed, [b.id])

  // b answers now; a is left out (it stopped answering its health check):
  // nothing a waited on earlier stays.
  lists[b.id] = [item('p2', { waiting: { cards: 2 } })]
  await inbox.refresh([b])
  assert.deepEqual(inbox.entries.map((e) => `${e.desktopName}/${e.sessionID}`), ['b/p2'])
  assert.deepEqual(inbox.failed, [])
})

test('answering a tool call goes to the desktop that asked, then reads it again', async () => {
  const a = paired('https://a.ts.net')
  const activity: Record<string, ActivitySnapshot> = {
    [a.id]: {
      running: [],
      pending_approvals: [{ request_id: 'r1', session_id: 'p1' }],
      queued_approvals: [{ approval_id: 'q1', session_id: 'p1' }],
    },
  }
  const { api, calls } = fakeInboxAPI({ [a.id]: [item('p1')] }, activity)
  const inbox = new InboxStore({ api })
  await inbox.refresh([a])
  const [entry] = inbox.entries
  const [queued, permission] = entry.approvals

  activity[a.id] = { running: [], pending_approvals: [{ request_id: 'r1', session_id: 'p1' }], queued_approvals: [] }
  assert.equal(await inbox.answer(a, queued, true), true)
  assert.equal(await inbox.answer(a, permission, false), true)
  assert.deepEqual(calls, [`${a.id} tok-${a.id} review q1 approve`, `${a.id} permission r1 p1 deny`])
  assert.deepEqual(inbox.entries[0].approvals.map((x) => x.id), ['r1'])
  assert.deepEqual(inbox.answering, {})
})

test('a tool call answered elsewhere, and one the desktop refuses', async () => {
  const a = paired('https://a.ts.net')
  const activity = { [a.id]: { running: [], pending_approvals: [], queued_approvals: [{ approval_id: 'q1', session_id: 'p1' }] } }
  for (const [err, failure] of [
    [new APIRequestError('closed', 409), 'gone'],
    [new APIRequestError('not found', 404), 'gone'],
    [new APIRequestError('forbidden', 403), 'failed'],
    [new TypeError('Load failed'), 'failed'],
  ] as const) {
    const { api } = fakeInboxAPI({ [a.id]: [item('p1')] }, activity)
    api.reviewApproval = async () => {
      throw err
    }
    const inbox = new InboxStore({ api })
    await inbox.refresh([a])
    assert.equal(await inbox.answer(a, inbox.entries[0].approvals[0], true), false)
    assert.equal(inbox.answerFailures.q1, failure)
    assert.deepEqual(inbox.answering, {})
  }
})
