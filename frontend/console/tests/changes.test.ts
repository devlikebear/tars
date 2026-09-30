import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import { turnCardAnchors, fileTreeRows } from '../src/lib/changes.ts'
import type * as ChangesModule from '../src/lib/stores/changes.svelte.ts'
import type { CheckpointDiff, CheckpointEntry, CheckpointScope } from '../src/lib/api/checkpoints.ts'

const { ChangesStore } = await compileSvelteModule<typeof ChangesModule>('src/lib/stores/changes.svelte.ts')

function entry(turnId: string, files: number, extra: Partial<CheckpointEntry> = {}): CheckpointEntry {
  return { turn_id: turnId, root: '/repo', shadow: 'abc', started_at: '2026-09-27T10:00:00Z', files, additions: files, deletions: 0, ...extra }
}

function fakeApi(turnsBySession: Record<string, CheckpointEntry[]>) {
  const calls = { list: [] as string[], diff: [] as string[] }
  const api = {
    calls,
    listCheckpoints: async (sessionId: string) => {
      calls.list.push(sessionId)
      return { session_id: sessionId, turns: turnsBySession[sessionId] ?? [] }
    },
    getCheckpointDiff: async (sessionId: string, turnId: string, options: { scope?: CheckpointScope; path?: string } = {}) => {
      calls.diff.push(`${sessionId}/${turnId}/${options.scope ?? ''}/${options.path ?? ''}`)
      if (turnId === 'broken') throw new Error('boom')
      return { turn_id: turnId, scope: options.scope ?? 'turn', root: '/repo', from: 'a', to: 'b', files: [] } satisfies CheckpointDiff
    },
  }
  return api
}

test('the card for a turn sits under the last message before the next user message', () => {
  const anchors = turnCardAnchors([
    { id: 'system-init', role: 'system' },
    { id: 'u1', role: 'user', sourceMessageId: 'turn-1' },
    { id: 'tool-1', role: 'tool' },
    { id: 'a1', role: 'assistant' },
    { id: 'u2', role: 'user' },
    { id: 'a2', role: 'assistant' },
    { id: 'u3', role: 'user', sourceMessageId: 'turn-3' },
  ])
  assert.deepEqual([...anchors], [['a1', 'turn-1'], ['u3', 'turn-3']])
})

test('load lists a session and keeps turns that changed files newest first', async () => {
  const api = fakeApi({ s1: [entry('t1', 2), entry('t2', 0), entry('t3', 1), entry('t4', 0, { skipped: 'too_large' })] })
  const store = new ChangesStore(api)
  await store.load('s1')
  assert.equal(store.sessionId, 's1')
  assert.deepEqual(store.changedTurns.map((t) => t.turn_id), ['t3', 't1'])
  assert.equal(store.selectedTurn?.turn_id, 't3')
  store.select('t1')
  assert.equal(store.selectedTurn?.turn_id, 't1')
  store.select('gone')
  assert.equal(store.selectedTurn?.turn_id, 't3', 'an unknown selection falls back to the latest')
  assert.equal(store.turn('t4')?.skipped, 'too_large')
  assert.equal(store.turn(undefined), undefined)
})

test('switching sessions clears turns, selection, and cached diffs', async () => {
  const api = fakeApi({ s1: [entry('t1', 1)], s2: [entry('t9', 1)] })
  const store = new ChangesStore(api)
  await store.load('s1')
  store.select('t1')
  await store.diff('t1')
  await store.load('s2')
  assert.equal(store.selectedTurnId, null)
  assert.deepEqual(store.turns.map((t) => t.turn_id), ['t9'])
  await store.load(null)
  assert.equal(store.sessionId, null)
  assert.deepEqual(store.turns, [])
})

test('diffs are fetched once per turn, scope, and path; refresh drops all but turn diffs', async () => {
  const api = fakeApi({ s1: [entry('t1', 1)] })
  const store = new ChangesStore(api)
  await store.load('s1')
  await store.diff('t1')
  await store.diff('t1')
  await store.diff('t1', 'session')
  await store.diff('t1', 'turn', 'a.txt')
  assert.deepEqual(api.calls.diff, ['s1/t1/turn/', 's1/t1/session/', 's1/t1/turn/a.txt'])
  const before = store.version
  await store.refresh()
  assert.ok(store.version > before)
  await store.diff('t1')
  await store.diff('t1', 'session')
  assert.equal(api.calls.diff.length, 4, 'only the session diff is fetched again')
})

test('a failed diff is not cached', async () => {
  const api = fakeApi({ s1: [entry('broken', 1)] })
  const store = new ChangesStore(api)
  await store.load('s1')
  await assert.rejects(store.diff('broken'))
  await assert.rejects(store.diff('broken'))
  assert.equal(api.calls.diff.length, 2)
  const empty = new ChangesStore(api)
  await assert.rejects(empty.diff('t1'), /no session/)
})

test('a checkpoint event shows at once, adopts a new session, and reloads the list', async () => {
  const api = fakeApi({ fresh: [entry('t1', 3)] })
  const store = new ChangesStore(api)
  store.applyEvent({ session_id: 'fresh', user_message_id: 't1', files: 3, additions: 5, deletions: 1 })
  assert.equal(store.sessionId, 'fresh')
  assert.equal(store.turn('t1')?.files, 3)
  assert.equal(store.turn('t1')?.additions, 5)
  store.applyEvent({ session_id: 'fresh', user_message_id: 't1', files: 3, additions: 6, deletions: 1 })
  assert.equal(store.turns.length, 1)
  assert.equal(store.turn('t1')?.additions, 6)
  store.applyEvent({ session_id: '', user_message_id: 't2', files: 1, additions: 1, deletions: 0 })
  assert.equal(store.turns.length, 1, 'an event without a session is ignored')
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.ok(api.calls.list.includes('fresh'))
  assert.equal(store.turn('t1')?.root, '/repo')
})

test('a failed list keeps the error and a stale response is dropped', async () => {
  let release: (value: { session_id: string; turns: CheckpointEntry[] }) => void = () => {}
  const api = {
    listCheckpoints: (sessionId: string) => {
      if (sessionId === 'bad') return Promise.reject(new Error('offline'))
      if (sessionId === 'slow') return new Promise<{ session_id: string; turns: CheckpointEntry[] }>((resolve) => { release = resolve })
      return Promise.resolve({ session_id: sessionId, turns: [entry('t1', 1)] })
    },
    getCheckpointDiff: async () => { throw new Error('unused') },
  }
  const store = new ChangesStore(api)
  await store.load('bad')
  assert.equal(store.error, 'offline')
  assert.equal(store.loading, false)
  const slow = store.load('slow')
  await store.load('fast')
  release({ session_id: 'slow', turns: [entry('old', 1)] })
  await slow
  assert.deepEqual(store.turns.map((t) => t.turn_id), ['t1'])
  assert.equal(store.error, '')
})

test('scope is shared by the panel and the cards', () => {
  const store = new ChangesStore(fakeApi({}))
  assert.equal(store.scope, 'turn')
  store.setScope('session')
  assert.equal(store.scope, 'session')
})

test('fileTreeRows groups files into folders, folding single-folder chains', () => {
  const files = [
    { path: 'README.md', additions: 1, deletions: 0 },
    { path: 'src/lib/a.ts', additions: 2, deletions: 1 },
    { path: 'src/lib/b.ts', additions: 3, deletions: 0 },
    { path: 'src/main.ts', additions: 0, deletions: 4 },
    { path: 'docs/deep/er/x.md', additions: 1, deletions: 1 },
  ]
  const rows = fileTreeRows(files)
  assert.deepEqual(
    rows.map((r) => `${'  '.repeat(r.depth)}${r.kind === 'dir' ? `${r.name}/ ${r.files} +${r.additions} -${r.deletions}` : r.name}`),
    [
      'docs/deep/er/ 1 +1 -1',
      '  x.md',
      'src/ 3 +5 -5',
      '  lib/ 2 +5 -1',
      '    a.ts',
      '    b.ts',
      '  main.ts',
      'README.md',
    ],
  )
  const folded = fileTreeRows(files, new Set(['src/lib', 'docs/deep/er']))
  assert.deepEqual(folded.map((r) => r.path), ['docs/deep/er', 'src', 'src/lib', 'src/main.ts', 'README.md'])
  assert.deepEqual(fileTreeRows([]), [])
})
