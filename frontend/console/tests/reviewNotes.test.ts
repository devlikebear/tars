import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import { splitReviewNotes } from '../src/lib/changes.ts'
import type * as ChangesModule from '../src/lib/stores/changes.svelte.ts'
import type { CheckpointEntry, RevertRequest, RevertResult } from '../src/lib/api/checkpoints.ts'

const { ChangesStore } = await compileSvelteModule<typeof ChangesModule>('src/lib/stores/changes.svelte.ts')

const turn: CheckpointEntry = { turn_id: 't1', root: '/repo', shadow: 'abc', started_at: '', files: 2, additions: 2, deletions: 2 }

function fakeApi(apply: RevertResult) {
  return {
    listCheckpoints: async (sessionId: string) => ({ session_id: sessionId, turns: [turn], reverts: [] }),
    getCheckpointDiff: async () => { throw new Error('unused') },
    revertCheckpoint: async (_s: string, _t: string, request: RevertRequest) =>
      request.apply ? apply : { ...apply, applied: false, revert_id: undefined },
    undoRevert: async () => ({ ...apply, applied: true }),
  }
}

function written(...paths: string[]): RevertResult {
  return { revert_id: 'r1', turn_id: 't1', scope: 'turn', applied: true, conflicts: 0, failed: 0, files: paths.map((path) => ({ path, outcome: 'write' })) }
}

test('the review-notes block folds off a stored message', () => {
  const block = '<review-notes>\nNotes from the review.\n\n1. a.txt, hunk h0\nComment: louder\n\n2. b.txt\nThe user reverted this change.\n</review-notes>'
  assert.deepEqual(splitReviewNotes(`please rework\n\n${block}`), { text: 'please rework', notes: block, count: 2 })
  assert.deepEqual(splitReviewNotes('plain text'), { text: 'plain text', notes: '', count: 0 })
  assert.equal(splitReviewNotes('mentions <review-notes> inline').count, 0)
})

test('notes are added, removed, and taken in the shape the server reads', async () => {
  const store = new ChangesStore(fakeApi(written('a.txt')))
  await store.load('s1')
  store.addNote({ turn_id: 't1', path: 'a.txt', hunk_id: 'h0', comment: 'louder', kind: 'comment' })
  store.addNote({ turn_id: 't1', path: 'b.txt', comment: 'drop it', kind: 'comment' })
  assert.equal(store.notes.length, 2)
  store.removeNote(store.notes[1].id)
  assert.deepEqual(store.takeNotes(), [{ turn_id: 't1', path: 'a.txt', hunk_id: 'h0', comment: 'louder', kind: 'comment' }])
  assert.deepEqual(store.notes, [], 'sent notes are cleared')
})

test('a revert leaves a note per reverted hunk, and undoing takes them back', async () => {
  const store = new ChangesStore(fakeApi(written('a.txt')))
  await store.load('s1')
  store.addNote({ turn_id: 't1', path: 'c.txt', comment: 'keep me', kind: 'comment' })
  await store.requestRevert('t1', 'turn', [{ path: 'a.txt', hunk_ids: ['h0', 'h2'] }])
  await store.confirmRevert()
  assert.deepEqual(store.notes.map((n) => [n.path, n.hunk_id, n.kind, n.revertId]), [
    ['c.txt', undefined, 'comment', undefined],
    ['a.txt', 'h0', 'revert', 'r1'],
    ['a.txt', 'h2', 'revert', 'r1'],
  ])
  await store.undo()
  assert.deepEqual(store.notes.map((n) => n.path), ['c.txt'])
})

test('a whole-turn or since revert notes the files it wrote', async () => {
  const store = new ChangesStore(fakeApi(written('a.txt', 'b.txt')))
  await store.load('s1')
  await store.requestRevert('t1', 'since')
  await store.confirmRevert()
  assert.deepEqual(store.notes.map((n) => [n.path, n.hunk_id]), [['a.txt', undefined], ['b.txt', undefined]])
  const switching = store.load('s2')
  assert.deepEqual(store.notes, [], 'another session starts with no notes')
  await switching
})

test('a revert that wrote nothing leaves no note', async () => {
  const store = new ChangesStore(fakeApi({ ...written(), revert_id: undefined }))
  await store.load('s1')
  await store.requestRevert('t1', 'turn', [{ path: 'a.txt' }])
  await store.confirmRevert()
  assert.deepEqual(store.notes, [])
})

test('notes taken for a send the server refused come back, once, for the same session only', async () => {
  const store = new ChangesStore(fakeApi(written('a.txt')))
  await store.load('s1')
  store.addNote({ turn_id: 't1', path: 'a.txt', comment: 'louder', kind: 'comment' })
  const drafts = [...store.notes]
  store.takeNotes()
  store.addNote({ turn_id: 't1', path: 'b.txt', comment: 'added meanwhile', kind: 'comment' })
  store.restoreNotes('s1', drafts)
  store.restoreNotes('s1', drafts)
  assert.deepEqual(store.notes.map((n) => n.path), ['a.txt', 'b.txt'], 'they go back first, not twice')
  store.restoreNotes('s2', [{ id: 'other', turn_id: 't9', path: 'z.txt' }])
  assert.deepEqual(store.notes.map((n) => n.path), ['a.txt', 'b.txt'], 'another session’s notes are not mixed in')
})
