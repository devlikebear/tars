import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'

const headerSource = readFileSync(new URL('../src/components/Header.svelte', import.meta.url), 'utf8')
const shellSource = readFileSync(new URL('../src/components/Shell.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Header renders a daily token budget chip backed by the usage today API', () => {
  assert.match(apiSource, /getTodayUsage/)
  assert.match(apiSource, /\/v1\/admin\/usage\/today/)
  assert.match(typesSource, /export type UsageToday/)
  assert.match(headerSource, /getTodayUsage/)
  assert.match(headerSource, /budgetVisible/)
  assert.match(headerSource, /budget-chip/)
  assert.match(headerSource, /usageToday\.level === 'error'/)
  assert.match(headerSource, /\/console\/analytics\?focus=today/)
  assert.match(shellSource, /<Header \{serverHealth\} \{unreadCount\} \{onUnreadChange\} \{onNavigate\} \/>/)
})

test('Header budget labels are localized', () => {
  assert.equal(en.header.budget.label, 'Daily token budget')
  assert.equal(ko.header.budget.label, '오늘 토큰 예산')
  assert.match(en.header.budget.title('12.3K', '200K', 6), /12\.3K of 200K/)
  assert.match(ko.header.budget.title('12.3K', '200K', 6), /오늘 토큰/)
})
