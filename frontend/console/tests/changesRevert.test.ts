import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as ChangesModule from '../src/lib/stores/changes.svelte.ts'
import type { CheckpointEntry, RevertEntry, RevertRequest, RevertResult } from '../src/lib/api/checkpoints.ts'

const { ChangesStore } = await compileSvelteModule<typeof ChangesModule>('src/lib/stores/changes.svelte.ts')

const turn: CheckpointEntry = { turn_id: 't1', root: '/repo', shadow: 'abc', started_at: '2026-09-27T10:00:00Z', files: 1, additions: 2, deletions: 2 }

function result(extra: Partial<RevertResult> = {}): RevertResult {
  return { turn_id: 't1', scope: 'turn', applied: false, conflicts: 0, failed: 0, files: [{ path: 'a.txt', outcome: 'write' }], ...extra }
}

// A 409 as requestJSON throws it.
function httpError(status: number, payload: Record<string, unknown>) {
  return Object.assign(new Error(String(payload.error ?? status)), { status, payload })
}

function fakeApi() {
  const calls: Array<{ kind: string; request?: RevertRequest; force?: boolean }> = []
  let reverts: RevertEntry[] = []
  const api = {
    calls,
    next: {
      preview: result() as RevertResult | Error,
      apply: result({ applied: true, revert_id: 'r1' }) as RevertResult | Error,
      undo: result({ applied: true, revert_id: 'r1' }) as RevertResult | Error,
    },
    setReverts(list: RevertEntry[]) { reverts = list },
    listCheckpoints: async (sessionId: string) => ({ session_id: sessionId, turns: [turn], reverts }),
    getCheckpointDiff: async () => { throw new Error('unused') },
    revertCheckpoint: async (_sessionId: string, _turnId: string, request: RevertRequest) => {
      calls.push({ kind: request.apply ? 'apply' : 'preview', request })
      const next = request.apply ? api.next.apply : api.next.preview
      if (next instanceof Error) throw next
      return next
    },
    undoRevert: async (_sessionId: string, _revertId: string, force = false) => {
      calls.push({ kind: 'undo', force })
      if (api.next.undo instanceof Error) throw api.next.undo
      return api.next.undo
    },
  }
  return api
}

async function loaded() {
  const api = fakeApi()
  const store = new ChangesStore(api)
  await store.load('s1')
  return { api, store }
}

test('a revert is previewed, confirmed, and then offered for undo', async () => {
  const { api, store } = await loaded()
  await store.requestRevert('t1', 'turn', [{ path: 'a.txt', hunk_ids: ['h1'] }])
  assert.equal(store.pending?.stage, 'confirm')
  assert.deepEqual(api.calls[0], { kind: 'preview', request: { scope: 'turn', files: [{ path: 'a.txt', hunk_ids: ['h1'] }] } })

  await store.confirmRevert()
  assert.equal(store.pending, null)
  assert.equal(store.last?.revertId, 'r1')
  assert.equal(store.last?.stage, 'done')
  assert.equal(api.calls[1].request?.apply, true)
  assert.equal(api.calls[1].request?.force, false)

  await store.undo()
  assert.equal(store.last?.stage, 'undone')
  assert.deepEqual(api.calls[2], { kind: 'undo', force: false })
  await store.undo()
  assert.equal(api.calls.length, 3, 'an undone revert is not undone twice')
  store.dismissLast()
  assert.equal(store.last, null)
})

test('a conflicting preview asks to force, and a conflicting apply comes back as one', async () => {
  const { api, store } = await loaded()
  const conflicted = result({ conflicts: 1, files: [{ path: 'a.txt', outcome: 'conflict', merged: '<<<<<<< current' }] })
  api.next.preview = conflicted
  await store.requestRevert('t1', 'turn')
  assert.equal(store.pending?.stage, 'conflict')

  api.next.preview = result()
  await store.requestRevert('t1', 'turn')
  api.next.apply = httpError(409, { code: 'revert_conflict', error: 'conflict', result: conflicted })
  await store.confirmRevert()
  assert.equal(store.pending?.stage, 'conflict')
  assert.equal(store.pending?.result?.files[0].merged, '<<<<<<< current')

  api.next.apply = result({ applied: true, revert_id: 'r2', files: [{ path: 'a.txt', outcome: 'write', forced: true }] })
  await store.confirmRevert(true)
  assert.equal(api.calls.at(-1)?.request?.force, true)
  assert.equal(store.last?.revertId, 'r2')
})

test('errors keep their code for the panel to word', async () => {
  const { api, store } = await loaded()
  api.next.preview = httpError(400, { code: 'bad_request', error: 'no hunk' })
  await store.requestRevert('t1', 'turn')
  assert.equal(store.pending?.stage, 'error')
  assert.equal(store.pending?.errorCode, 'bad_request')
  store.cancelRevert()
  assert.equal(store.pending, null)

  api.next.preview = result()
  await store.requestRevert('t1', 'turn')
  api.next.apply = httpError(409, { code: 'turn_in_progress', error: 'busy' })
  await store.confirmRevert()
  assert.equal(store.pending?.stage, 'error')
  assert.equal(store.pending?.errorCode, 'turn_in_progress')

  api.next.preview = result()
  await store.requestRevert('t1', 'turn')
  api.next.apply = result({ applied: true, revert_id: 'r3' })
  await store.confirmRevert()
  api.next.undo = httpError(409, { code: 'revert_conflict', error: 'conflict', result: result({ conflicts: 1 }) })
  await store.undo()
  assert.equal(store.last?.stage, 'undoConflict')
  api.next.undo = new Error('offline')
  await store.undo(true)
  assert.equal(store.last?.stage, 'error')
  assert.equal(store.last?.error, 'offline')
})

test('confirm and undo do nothing without something to act on', async () => {
  const { api, store } = await loaded()
  await store.confirmRevert()
  await store.undo()
  assert.equal(api.calls.length, 0)
  const empty = new ChangesStore(fakeApi())
  await empty.requestRevert('t1', 'turn')
  assert.equal(empty.pending, null)
})

test('a newer request replaces a pending preview', async () => {
  const { api, store } = await loaded()
  const first = store.requestRevert('t1', 'turn')
  store.cancelRevert()
  await first
  assert.equal(store.pending, null, 'a cancelled preview does not come back')
  api.next.preview = httpError(500, { error: 'late' })
  const failed = store.requestRevert('t1', 'turn')
  store.cancelRevert()
  await failed
  assert.equal(store.pending, null)
})

test('reverted marks come from live reverts in the list', async () => {
  const { api, store } = await loaded()
  api.setReverts([
    { id: 'r1', turn_id: 't1', scope: 'turn', at: '', targets: [{ path: 'a.txt', hunk_ids: ['h1'] }], files: ['a.txt'] },
    { id: 'r2', turn_id: 't1', scope: 'turn', at: '', targets: [{ path: 'b.txt' }], files: ['b.txt'] },
    { id: 'r3', turn_id: 't1', scope: 'turn', at: '', targets: [{ path: 'c.txt' }], files: ['c.txt'], undone_at: '2026-09-27T11:00:00Z' },
    { id: 'r4', turn_id: 't1', scope: 'since', at: '', targets: [{ path: 'd.txt' }], files: ['d.txt'] },
  ])
  await store.refresh()
  assert.equal(store.isReverted('t1', 'a.txt', 'h1'), true)
  assert.equal(store.isReverted('t1', 'a.txt', 'h0'), false)
  assert.equal(store.isReverted('t1', 'a.txt'), false, 'a hunk revert is not a whole-file one')
  assert.equal(store.isReverted('t1', 'b.txt'), true)
  assert.equal(store.isReverted('t1', 'b.txt', 'h3'), true)
  assert.equal(store.isReverted('t1', 'c.txt'), false, 'undone')
  assert.equal(store.isReverted('t1', 'd.txt'), false, 'since reverts span turns')
  assert.equal(store.isReverted('t2', 'b.txt'), false)

  const switching = store.load('s2')
  assert.deepEqual(store.reverts, [], 'another session starts with no marks')
  await switching
})
