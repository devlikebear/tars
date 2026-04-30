import { strict as assert } from 'node:assert'
import test from 'node:test'

import { en } from '../src/i18n/en.ts'
import { ko } from '../src/i18n/ko.ts'
import { normalizeLocale, resolveInitialLocale, STORAGE_KEY } from '../src/i18n/locale.ts'

test('console locale resolution prefers stored locale, then browser language, then English', () => {
  assert.equal(STORAGE_KEY, 'tars_console_locale')
  assert.equal(normalizeLocale('ko-KR'), 'ko')
  assert.equal(normalizeLocale('en-US'), 'en')
  assert.equal(normalizeLocale('ja-JP'), '')
  assert.equal(resolveInitialLocale('ko', 'en-US'), 'ko')
  assert.equal(resolveInitialLocale(null, 'ko-KR'), 'ko')
  assert.equal(resolveInitialLocale(null, 'fr-FR'), 'en')
})

test('English and Korean translation maps cover first-pass console surfaces', () => {
  assert.deepEqual(Object.keys(ko.nav.items), Object.keys(en.nav.items))
  assert.deepEqual(Object.keys(ko.header.filters), Object.keys(en.header.filters))
  assert.deepEqual(Object.keys(ko.sessions.filters), Object.keys(en.sessions.filters))
  assert.deepEqual(Object.keys(ko.memory.tabs), Object.keys(en.memory.tabs))
  assert.deepEqual(Object.keys(ko.tasks.stats), Object.keys(en.tasks.stats))

  assert.equal(en.nav.items.chat, 'Chat')
  assert.equal(ko.nav.items.chat, '채팅')
  assert.equal(en.header.title, 'Console')
  assert.equal(ko.header.title, '콘솔')
})
