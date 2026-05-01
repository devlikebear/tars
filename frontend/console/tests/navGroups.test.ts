import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')

test('Console nav groups routes into Work, Operate, and Setup sections', () => {
  assert.match(navSource, /interface NavGroup/)
  assert.match(navSource, /const groups: NavGroup\[\]/)
  assert.match(navSource, /id: 'work'/)
  assert.match(navSource, /id: 'operate'/)
  assert.match(navSource, /id: 'setup'/)
  assert.match(navSource, /'chat'[\s\S]*'plans'[\s\S]*'memory'[\s\S]*'sysprompt'[\s\S]*'extensions'/)
  assert.match(navSource, /'agentruntime'[\s\S]*'ops'[\s\S]*'cron'[\s\S]*'logs'[\s\S]*'analytics'[\s\S]*'pulse'[\s\S]*'reflection'/)
  assert.match(navSource, /'config'/)
  assert.equal(en.nav.groups.work, 'Work')
  assert.equal(en.nav.items.plans, 'Plans')
  assert.equal(en.nav.items.sysprompt, 'System Prompt')
  assert.equal(en.nav.items.ops, 'Approvals')
  assert.equal(en.nav.items.cron, 'Cron')
  assert.equal(en.nav.items.logs, 'Logs')
  assert.equal(en.nav.items.analytics, 'Analytics')
  assert.equal(ko.nav.items.chat, '채팅')
  assert.equal(ko.nav.items.plans, '계획')
  assert.match(navSource, /nav-group-label/)
  assert.match(navSource, /groupLabel\(group\.id\)/)
  assert.match(navSource, /itemLabel\(item\.id\)/)
})
