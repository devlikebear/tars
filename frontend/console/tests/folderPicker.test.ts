import test from 'node:test'
import assert from 'node:assert/strict'

import { PickerRequests, arrangePickerEntries, browseErrorReason, isHiddenFolder, resolvePickerPath } from '../src/lib/folderPicker.ts'

const home = '/w/home'

test('an absolute path is used as typed, trailing slashes dropped', () => {
  assert.deepEqual(resolvePickerPath('/w/repo/.claude/worktrees/', home), { ok: true, path: '/w/repo/.claude/worktrees' })
  assert.deepEqual(resolvePickerPath('  /w/repo  ', home), { ok: true, path: '/w/repo' })
  assert.deepEqual(resolvePickerPath('/', home), { ok: true, path: '/' })
})

test('a pasted path in quotes loses its quotes', () => {
  assert.deepEqual(resolvePickerPath('"/w/my repo"', home), { ok: true, path: '/w/my repo' })
  assert.deepEqual(resolvePickerPath("'/w/my repo'", home), { ok: true, path: '/w/my repo' })
})

test('a leading ~ is the home folder', () => {
  assert.deepEqual(resolvePickerPath('~', home), { ok: true, path: home })
  assert.deepEqual(resolvePickerPath('~/', home), { ok: true, path: home })
  assert.deepEqual(resolvePickerPath('~/workspace/tars', home), { ok: true, path: '/w/home/workspace/tars' })
  assert.deepEqual(resolvePickerPath('~/workspace/', '/w/home/'), { ok: true, path: '/w/home/workspace' })
})

test('~ follows a Windows home folder', () => {
  assert.deepEqual(resolvePickerPath('~\\src\\tars', 'C:\\w\\home'), { ok: true, path: 'C:\\w\\home\\src\\tars' })
  assert.deepEqual(resolvePickerPath('C:\\w\\repo\\', home), { ok: true, path: 'C:\\w\\repo' })
  assert.deepEqual(resolvePickerPath('C:\\', home), { ok: true, path: 'C:\\' })
})

test('empty, relative, and other-user paths are refused before any request', () => {
  assert.deepEqual(resolvePickerPath('   ', home), { ok: false, reason: 'empty' })
  assert.deepEqual(resolvePickerPath('workspace/tars', home), { ok: false, reason: 'relative' })
  assert.deepEqual(resolvePickerPath('./tars', home), { ok: false, reason: 'relative' })
  assert.deepEqual(resolvePickerPath('~other/tars', home), { ok: false, reason: 'relative' })
  // Home unknown (the first listing failed): ~ cannot be expanded.
  assert.deepEqual(resolvePickerPath('~/tars', ''), { ok: false, reason: 'relative' })
})

test('browse errors map to a reason by status', () => {
  assert.equal(browseErrorReason(404), 'notFound')
  assert.equal(browseErrorReason(400), 'notDirectory')
  assert.equal(browseErrorReason(403), 'unreadable')
  assert.equal(browseErrorReason(500), 'failed')
  assert.equal(browseErrorReason(undefined), 'failed')
})

test('dot folders are hidden', () => {
  assert.equal(isHiddenFolder('.claude'), true)
  assert.equal(isHiddenFolder('claude'), false)
  assert.equal(isHiddenFolder('..'), false)
})

const entries = ['.cache', 'Documents', '.claude', 'workspace', 'Work', '.config'].map((name) => ({ name, path: `/w/home/${name}` }))

test('hidden folders go to their own group, order kept', () => {
  const { shown, hidden } = arrangePickerEntries(entries, '')
  assert.deepEqual(shown.map((e) => e.name), ['Documents', 'workspace', 'Work'])
  assert.deepEqual(hidden.map((e) => e.name), ['.cache', '.claude', '.config'])
})

test('the filter matches part of the name, ignoring case, in both groups', () => {
  const work = arrangePickerEntries(entries, '  WOR ')
  assert.deepEqual(work.shown.map((e) => e.name), ['workspace', 'Work'])
  assert.deepEqual(work.hidden.map((e) => e.name), [])

  const c = arrangePickerEntries(entries, 'c')
  assert.deepEqual(c.shown.map((e) => e.name), ['Documents', 'workspace'])
  assert.deepEqual(c.hidden.map((e) => e.name), ['.cache', '.claude', '.config'])

  const none = arrangePickerEntries(entries, 'zzz')
  assert.deepEqual(none, { shown: [], hidden: [] })
})

test('a path typed while the first listing loads survives that listing', () => {
  const requests = new PickerRequests()
  const home = requests.start('')
  // The user types a path before the home listing comes back…
  const typed = '/w/repo'
  // …and the home listing lands before Enter: it shows, but the box keeps the typing.
  assert.deepEqual(requests.settle(home, typed, '/w/home'), { current: true, pathInput: typed })
  const submitted = requests.start(typed)
  assert.deepEqual(requests.settle(submitted, typed, '/w/repo'), { current: true, pathInput: '/w/repo' })
})

test('a listing that comes back after a newer request is dropped', () => {
  const requests = new PickerRequests()
  const home = requests.start('')
  const submitted = requests.start('/w/repo')
  // Responses arrive out of order: the submitted path first, then home.
  assert.deepEqual(requests.settle(submitted, '/w/repo', '/w/repo'), { current: true, pathInput: '/w/repo' })
  assert.deepEqual(requests.settle(home, '/w/repo', '/w/home'), { current: false })
  assert.equal(requests.isCurrent(home), false)
  assert.equal(requests.isCurrent(submitted), true)
})

test('an untouched path box follows the listing, normalized path included', () => {
  const requests = new PickerRequests()
  const home = requests.start('')
  assert.deepEqual(requests.settle(home, '', '/w/home'), { current: true, pathInput: '/w/home' })
  // A click into a subfolder: the box still shows the previous folder.
  const child = requests.start('/w/home')
  assert.deepEqual(requests.settle(child, '/w/home', '/w/home/src'), { current: true, pathInput: '/w/home/src' })
})
