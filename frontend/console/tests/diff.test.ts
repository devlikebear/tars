import test from 'node:test'
import assert from 'node:assert/strict'

import { pairDiffLines, parsePatch, parseUnifiedDiff } from '../src/lib/diff.ts'

const twoFiles = [
  'diff --git a/src/a.sql b/src/a.sql',
  'index 1111111..2222222 100644',
  '--- a/src/a.sql',
  '+++ b/src/a.sql',
  '@@ -1,4 +1,4 @@ create',
  ' select 1;',
  '--- old comment',
  '+++ new comment',
  ' select 2;',
  ' select 3;',
  '@@ -10 +10,2 @@',
  '-last',
  '+last',
  '+more',
  '\\ No newline at end of file',
  'diff --git a/old name.txt b/new name.txt',
  'similarity index 90%',
  'rename from old name.txt',
  'rename to new name.txt',
  '--- a/old name.txt',
  '+++ b/new name.txt',
  '@@ -2,0 +3 @@',
  '+added',
  'diff --git a/logo.png b/logo.png',
  'Binary files a/logo.png and b/logo.png differ',
  '',
].join('\n')

test('parsePatch splits files and reads hunks by their line counts', () => {
  const files = parsePatch(twoFiles)
  assert.deepEqual(files.map((f) => f.path), ['src/a.sql', 'new name.txt', 'logo.png'])

  const [sql, renamed, logo] = files
  assert.equal(sql.hunks.length, 2)
  assert.deepEqual(sql.hunks[0], { index: 0, oldStart: 1, oldLines: 4, newStart: 1, newLines: 4, header: '@@ -1,4 +1,4 @@ create' })
  assert.deepEqual(sql.hunks[1], { index: 1, oldStart: 10, oldLines: 1, newStart: 10, newLines: 2, header: '@@ -10 +10,2 @@' })
  // A deleted "-- old comment" is "--- old comment" in the patch; inside a
  // hunk it is a line, not a file header.
  const del = sql.lines.find((l) => l.kind === 'del')
  const add = sql.lines.find((l) => l.kind === 'add')
  assert.deepEqual(del, { kind: 'del', oldLine: 2, text: '-- old comment', hunk: 0 })
  assert.deepEqual(add, { kind: 'add', newLine: 2, text: '++ new comment', hunk: 0 })
  assert.equal(sql.lines.at(-1)?.kind, 'meta')
  assert.equal(sql.lines.at(-1)?.hunk, 1)

  assert.equal(renamed.oldPath, 'old name.txt')
  assert.equal(renamed.newPath, 'new name.txt')
  assert.deepEqual(renamed.lines.at(-1), { kind: 'add', newLine: 3, text: 'added', hunk: 0 })

  assert.equal(logo.binary, true)
  assert.equal(logo.lines.length, 0)
})

test('added and deleted files take their path from the side that exists', () => {
  const files = parsePatch([
    'diff --git a/gone.txt b/gone.txt',
    'deleted file mode 100644',
    '--- a/gone.txt',
    '+++ /dev/null',
    '@@ -1 +0,0 @@',
    '-bye',
    'diff --git a/new.txt b/new.txt',
    'new file mode 100644',
    '--- /dev/null',
    '+++ b/new.txt',
    '@@ -0,0 +1 @@',
    '+hi',
  ].join('\n'))
  assert.deepEqual(files.map((f) => [f.path, f.oldPath, f.newPath]), [
    ['gone.txt', 'gone.txt', undefined],
    ['new.txt', undefined, 'new.txt'],
  ])
})

test('a bare hunk without file headers still parses', () => {
  const lines = parseUnifiedDiff('@@ -1,2 +1,2 @@\n a\n-b\n+c\n')
  assert.deepEqual(lines.map((l) => l.kind), ['hunk', 'context', 'del', 'add'])
  assert.deepEqual(parseUnifiedDiff(''), [])
  assert.deepEqual(parseUnifiedDiff(undefined), [])
})

test('an empty context line (stripped trailing space) still counts', () => {
  const [file] = parsePatch('@@ -1,3 +1,3 @@\n a\n\n-c\n+d\n')
  assert.deepEqual(file.lines.map((l) => [l.kind, l.oldLine, l.newLine]), [
    ['hunk', undefined, undefined],
    ['context', 1, 1],
    ['context', 2, 2],
    ['del', 3, undefined],
    ['add', undefined, 3],
  ])
})

test('pairDiffLines lines deletions up with the additions that replace them', () => {
  const pairs = pairDiffLines(parseUnifiedDiff('@@ -1,3 +1,2 @@\n a\n-b\n-c\n+d\n'))
  assert.deepEqual(pairs.map((p) => [p.left?.kind, p.left?.text, p.right?.kind, p.right?.text]), [
    ['hunk', '@@ -1,3 +1,2 @@', 'hunk', '@@ -1,3 +1,2 @@'],
    ['context', 'a', 'context', 'a'],
    ['del', 'b', 'add', 'd'],
    ['del', 'c', undefined, undefined],
  ])
})
