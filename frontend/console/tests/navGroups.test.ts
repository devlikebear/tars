import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'
import { navGroups, visibleNavGroups } from '../src/lib/navGroups.ts'
import { pageEntries } from '../src/lib/commands.ts'
import { resolveRoute } from '../src/lib/router.ts'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')

const ids = (groups: typeof navGroups) => groups.map((g) => [g.id, g.items.map((i) => i.view)])

test('the nav is grouped into Work, Build, and System (#968)', () => {
  assert.deepEqual(ids(navGroups), [
    ['work', ['chat']],
    ['build', ['agentruntime', 'memory', 'extensions', 'sysprompt']],
    ['system', ['ops', 'pulse', 'reflection', 'cron', 'logs', 'analytics', 'config']],
  ])
  for (const locale of [en, ko]) {
    for (const group of navGroups) assert.ok(locale.nav.groups[group.id], `${group.id} label`)
    for (const item of navGroups.flatMap((g) => g.items)) assert.ok(locale.nav.items[item.label], `${item.label} label`)
  }
  assert.equal(en.nav.groups.build, 'Build')
  assert.equal(ko.nav.groups.system, '시스템')
})

test('every nav path is the palette path and resolves to its own view', () => {
  for (const item of navGroups.flatMap((g) => g.items)) {
    const page = pageEntries.find((p) => p.view === item.view)
    assert.equal(item.path, page?.path, item.view)
    assert.equal(resolveRoute(item.path).view, item.view, item.path)
  }
})

// App.svelte renders admin-only views only for admin roles. A nav link the
// user role cannot open would silently land on Home.
test('the user role only sees pages it can open', () => {
  const userViews = visibleNavGroups('user').flatMap((g) => g.items.map((i) => i.view))
  assert.deepEqual(userViews, ['chat', 'agentruntime', 'memory', 'sysprompt'])
  for (const view of userViews) {
    assert.equal(pageEntries.find((p) => p.view === view)?.adminOnly, false, view)
  }
  assert.equal(visibleNavGroups('admin'), navGroups)
  assert.deepEqual(visibleNavGroups('user').map((g) => g.id), ['work', 'build'], 'empty groups are dropped')
})

test('Nav.svelte renders the shared groups and highlights by resolved route', () => {
  assert.match(navSource, /visibleNavGroups\(authRole\)/)
  assert.match(navSource, /resolveRoute\(currentPath\)\.view/)
  assert.match(navSource, /groupLabel\(group\.id\)/)
  assert.match(navSource, /itemLabel\(item\.label\)/)
  // Aliases highlight the same item as the canonical path.
  assert.equal(resolveRoute('/console/ops').view, 'ops')
  assert.equal(resolveRoute('/console/sessions').view, 'chat')
})
