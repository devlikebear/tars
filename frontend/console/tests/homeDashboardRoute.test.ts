import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/components/Home.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')

test('/console resolves to Home while Chat stays on /console/chat', () => {
  assert.deepEqual(resolveRoute('/console'), { view: 'home' })
  assert.deepEqual(resolveRoute('/console/'), { view: 'home' })
  assert.deepEqual(resolveRoute('/console/chat'), { view: 'chat' })
  assert.deepEqual(resolveRoute('/console/chat/session-1'), { view: 'chat', sessionId: 'session-1' })
})

test('App renders Home for the console entry route', () => {
  assert.match(appSource, /import Home from '\.\/components\/Home\.svelte'/)
  assert.match(appSource, /let route = \$state<Route>\(\{ view: 'home' \}\)/)
  assert.match(appSource, /route\.view === 'home'/)
  assert.match(appSource, /<Home onNavigate=\{navigate\} \/>/)
})

test('Home dashboard exposes system status, sessions, notifications, and actions', () => {
  assert.match(homeSource, /getOpsStatus/)
  assert.match(homeSource, /listSessions/)
  assert.match(homeSource, /getSyspromptFile/)
  assert.match(homeSource, /getConfig/)
  assert.match(homeSource, /getSessionTasks/)
  assert.match(homeSource, /Active sessions/)
  assert.match(homeSource, /Disk pressure/)
  assert.match(homeSource, /Recent notifications/)
  assert.match(homeSource, /Recommended actions/)
  assert.match(homeSource, /Continue working on/)
  assert.doesNotMatch(homeSource, /ChatPanel/)
})

test('Chat nav item is inactive on Home', () => {
  assert.doesNotMatch(navSource, /current === '\/console'/)
  assert.match(navSource, /current\.startsWith\('\/console\/chat'\)/)
})
