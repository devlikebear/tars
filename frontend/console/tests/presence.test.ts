import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  PRESENCE_ARRIVAL_HIDDEN_MS,
  PRESENCE_SEND_INTERVAL_MS,
  presenceShouldSendOnInput,
  presenceShouldSendOnStart,
  presenceShouldSendOnVisible,
} from '../src/lib/presence.ts'

test('presenceShouldSendOnVisible only fires past the arrival threshold', () => {
  assert.equal(presenceShouldSendOnVisible(0), false)
  assert.equal(presenceShouldSendOnVisible(PRESENCE_ARRIVAL_HIDDEN_MS - 1), false)
  assert.equal(presenceShouldSendOnVisible(PRESENCE_ARRIVAL_HIDDEN_MS), true)
  assert.equal(presenceShouldSendOnVisible(PRESENCE_ARRIVAL_HIDDEN_MS * 10), true)
  // A quick tab switch (well under the threshold) must never count.
  assert.equal(presenceShouldSendOnVisible(60_000), false)
  // An unknown-duration hidden start (pendingFirstVisible) always passes.
  assert.equal(presenceShouldSendOnVisible(Infinity), true)
})

// presenceShouldSendOnStart is the regression test for the review finding:
// a console opened as a background/hidden tab (a browser restoring a
// session, or a desktop shell window created hidden at login) must not
// send a presence signal on open — only its first real visibilitychange
// counts as an arrival.
test('presenceShouldSendOnStart only sends immediately when the tab is already visible', () => {
  assert.equal(presenceShouldSendOnStart(false), true)
  assert.equal(presenceShouldSendOnStart(true), false)
})

test('presenceShouldSendOnInput throttles to the configured interval', () => {
  const now = 1_000_000
  assert.equal(presenceShouldSendOnInput(now - PRESENCE_SEND_INTERVAL_MS + 1, now), false)
  assert.equal(presenceShouldSendOnInput(now - PRESENCE_SEND_INTERVAL_MS, now), true)
  assert.equal(presenceShouldSendOnInput(now - PRESENCE_SEND_INTERVAL_MS - 1, now), true)
  // Never having sent before (lastSentAt=0) is always past the interval.
  assert.equal(presenceShouldSendOnInput(0, now), true)
})

test('presence module posts to the dedicated endpoint, never a session id or message text', () => {
  const source = readFileSync(new URL('../src/lib/presence.ts', import.meta.url), 'utf8')
  assert.match(source, /\/v1\/console\/presence/)
  assert.match(source, /method: 'POST'/)
  assert.doesNotMatch(source, /session_?id/i)
})

test('startConsolePresence gates its immediate signal on presenceShouldSendOnStart', () => {
  const source = readFileSync(new URL('../src/lib/presence.ts', import.meta.url), 'utf8')
  const startFn = source.slice(source.indexOf('export function startConsolePresence'))
  assert.match(startFn, /presenceShouldSendOnStart\(document\.hidden\)/)
  assert.match(startFn, /pendingFirstVisible = true/)
})

test('App wires console presence independently of the companion feature', () => {
  const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
  assert.match(appSource, /startConsolePresence/)
  assert.match(appSource, /stopConsolePresence/)
  // Started from loadConsoleNotifications, not from inside CompanionPet —
  // the companion config value must not gate it.
  const companionSource = readFileSync(new URL('../src/components/CompanionPet.svelte', import.meta.url), 'utf8')
  assert.doesNotMatch(companionSource, /startConsolePresence/)
})
