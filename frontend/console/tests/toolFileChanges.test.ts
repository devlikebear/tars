import test from 'node:test'
import assert from 'node:assert/strict'

import { parseUnifiedDiff } from '../src/lib/diff.ts'
import {
  type ToolFileChange,
  fileChangeFromEvent,
  fileChangePatch,
  fileChangeStatus,
  mergeToolFileChange,
  totalFileChanges,
} from '../src/lib/toolFileChanges.ts'

test('fileChangeFromEvent reads a file_change event and ignores ones without a path', () => {
  assert.equal(fileChangeFromEvent({ path: '  ' }), null)
  assert.equal(fileChangeFromEvent({}), null)
  assert.deepEqual(fileChangeFromEvent({ path: 'img.png', op: 'modify', binary: true, additions: -1 }), {
    path: 'img.png',
    op: 'modify',
    additions: 0,
    deletions: 0,
    binary: true,
  })
  const change = fileChangeFromEvent({
    path: 'a.txt',
    op: 'create',
    additions: 2,
    deletions: 0,
    truncated: true,
    hunks: [{ old_start: 0, old_lines: 0, new_start: 1, new_lines: 2, lines: ['+one', '+two'] }],
  })
  assert.equal(change?.op, 'create')
  assert.equal(change?.truncated, true)
  assert.equal(change?.hunks?.length, 1)
  assert.equal(fileChangeFromEvent({ path: 'x' })?.op, 'modify')
  assert.equal(fileChangeFromEvent({ path: 'x', op: 'delete' })?.op, 'delete')
  assert.equal(fileChangeFromEvent({ path: 'x', op: 'rename' })?.op, 'modify')
})

test('mergeToolFileChange keeps one entry per path, latest wins', () => {
  const a1: ToolFileChange = { path: 'a', op: 'create', additions: 1, deletions: 0 }
  const b: ToolFileChange = { path: 'b', op: 'modify', additions: 2, deletions: 1 }
  const a2: ToolFileChange = { path: 'a', op: 'modify', additions: 3, deletions: 1 }
  const list = mergeToolFileChange(mergeToolFileChange(mergeToolFileChange(undefined, a1), b), a2)
  assert.deepEqual(list.map((c) => c.path), ['a', 'b'])
  assert.equal(list[0].additions, 3)
  assert.deepEqual(totalFileChanges(list), { files: 2, additions: 5, deletions: 2 })
  assert.deepEqual(totalFileChanges(undefined), { files: 0, additions: 0, deletions: 0 })
})

test('fileChangePatch renders hunks the shared diff parser reads', () => {
  const patch = fileChangePatch({
    path: 'a.txt',
    op: 'modify',
    additions: 2,
    deletions: 1,
    hunks: [{ old_start: 1, old_lines: 3, new_start: 1, new_lines: 4, lines: [' a', '-b', '+B', ' c', '+d'] }],
  })
  const lines = parseUnifiedDiff(patch)
  assert.deepEqual(lines.map((l) => l.kind), ['hunk', 'context', 'del', 'add', 'context', 'add'])
  assert.equal(lines[3].newLine, 2)
  assert.equal(lines[5].newLine, 4)
  assert.equal(fileChangePatch({ path: 'x', op: 'modify', additions: 0, deletions: 0 }), '')
})

test('fileChangeStatus maps ops onto the Changes panel status words', () => {
  assert.equal(fileChangeStatus('create'), 'added')
  assert.equal(fileChangeStatus('modify'), 'modified')
  assert.equal(fileChangeStatus('delete'), 'deleted')
  assert.equal(fileChangeStatus('rename'), 'rename')
})
