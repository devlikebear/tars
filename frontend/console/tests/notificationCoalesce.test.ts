import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const headerSource = readFileSync(new URL('../src/components/Header.svelte', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/components/Home.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('coalesced notification updates do not increment unread badges', () => {
  assert.match(typesSource, /occurrences\?: number/)
  assert.match(typesSource, /coalesced\?: boolean/)
  assert.match(appSource, /if \(!event\.coalesced\) unreadCount\+\+/)
  assert.match(homeSource, /if \(!event\.coalesced\) unreadCount\+\+/)
})

test('notification panels surface grouped occurrence counts and latest timestamp', () => {
  assert.match(headerSource, /notificationTimestamp\(item\)/)
  assert.match(headerSource, /x\{item\.occurrences\}/)
  assert.match(homeSource, /notificationTimestamp\(item\)/)
  assert.match(homeSource, /x\{item\.occurrences\}/)
})
