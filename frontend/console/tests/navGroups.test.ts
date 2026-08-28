import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')

test('Console nav is slimmed to chat, approvals, logs, pulse, and config', () => {
  assert.match(navSource, /interface NavGroup/)
  assert.match(navSource, /const groups: NavGroup\[\]/)
  assert.match(navSource, /id: 'work'/)
  assert.match(navSource, /id: 'operate'/)
  assert.match(navSource, /id: 'setup'/)
  assert.match(navSource, /'chat'[\s\S]*'ops'[\s\S]*'logs'[\s\S]*'pulse'[\s\S]*'config'/)
  assert.equal(en.nav.groups.work, 'Work')
  assert.equal(en.nav.items.ops, 'Approvals')
  assert.equal(en.nav.items.logs, 'Logs')
  assert.equal(ko.nav.items.chat, '채팅')
  assert.match(navSource, /nav-group-label/)
  assert.match(navSource, /groupLabel\(group\.id\)/)
  assert.match(navSource, /itemLabel\(item\.id\)/)
})

// #931 freeze: these pages keep their routes and stay reachable by URL, but the
// nav no longer advertises them.
test('nav hides the frozen long-tail pages without deleting their routes', () => {
  for (const hidden of [
    'lineage',
    'plans',
    'memory',
    'sysprompt',
    'extensions',
    'agentruntime',
    'channels',
    'cron',
    'analytics',
    'reflection',
  ]) {
    assert.doesNotMatch(navSource, new RegExp(`id: '${hidden}', path:`), `${hidden} must not be a nav item`)
  }
})

test('user role still sees a non-empty nav after the slim down', () => {
  assert.match(navSource, /const userVisibleItems = new Set<NavItemId>\(\[\s*'chat',\s*'pulse',\s*\]\)/)
})
