import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'

const headerSource = readFileSync(new URL('../src/components/Header.svelte', import.meta.url), 'utf8')
const sessionSidebarSource = readFileSync(new URL('../src/components/SessionSidebar.svelte', import.meta.url), 'utf8')

test('Header auth actions are localized', () => {
  assert.equal(en.header.auth.signOut, 'Sign out')
  assert.equal(ko.header.auth.signOut, '로그아웃')
  assert.doesNotMatch(headerSource, />Sign out</)
  assert.match(headerSource, /\$t\.header\.auth\.signOut/)
})

test('Session sidebar icon action buttons have explicit accessible labels', () => {
  assert.match(sessionSidebarSource, /aria-label=\{\$t\.sessions\.actions\.more\}/)
  assert.match(sessionSidebarSource, /\$t\.sessions\.actions\.rename/)
  assert.match(sessionSidebarSource, /\$t\.sessions\.actions\.autoTitle/)
  assert.match(sessionSidebarSource, /\$t\.sessions\.actions\.compact/)
  assert.match(sessionSidebarSource, /deleteConfirmId === session\.id \? \$t\.sessions\.actions\.confirm : \$t\.sessions\.actions\.delete/)
})
