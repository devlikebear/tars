import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/components/Home.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const enSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('/console resolves to Home while Chat stays on /console/chat', () => {
  assert.deepEqual(resolveRoute('/console'), { view: 'home' })
  assert.deepEqual(resolveRoute('/console/'), { view: 'home' })
  assert.deepEqual(resolveRoute('/console/chat'), { view: 'chat' })
  assert.deepEqual(resolveRoute('/console/chat/session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.deepEqual(resolveRoute('/console/chat?session=session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.doesNotMatch(readFileSync(new URL('../src/lib/router.ts', import.meta.url), 'utf8'), /http:\/\/tars\.local/)
})

test('App renders Home for the console entry route', () => {
  assert.match(appSource, /import Home from '\.\/components\/Home\.svelte'/)
  assert.match(appSource, /let route = \$state<Route>\(\{ view: 'home' \}\)/)
  assert.match(appSource, /route\.view === 'home'/)
  assert.match(appSource, /<Home onNavigate=\{navigate\} \/>/)
  assert.doesNotMatch(appSource, /import Chat from '\.\/components\/Chat\.svelte'/)
})

test('Home dashboard exposes system status, sessions, notifications, and actions', () => {
  assert.match(homeSource, /getServerStatus/)
  assert.match(homeSource, /getOpsStatus/)
  assert.match(homeSource, /getGlobalPlans/)
  assert.match(homeSource, /listAgentRuntimeRuns/)
  assert.match(homeSource, /listCronJobs/)
  assert.match(homeSource, /listSessions/)
  assert.match(homeSource, /getSyspromptFile/)
  assert.match(homeSource, /getConfig/)
  assert.match(homeSource, /getSessionTasks/)
  assert.match(homeSource, /\$t\.home\.title/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.activePlans/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.agentRuns/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.cronJobs/)
  assert.match(homeSource, /\$t\.home\.delivery\.title/)
  assert.match(homeSource, /\$t\.home\.sessions\.title/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.diskPressure/)
  assert.match(homeSource, /\$t\.home\.notifications\.title/)
  assert.match(homeSource, /\$t\.home\.recommendations\.title/)
  assert.match(homeSource, /\$t\.home\.continue\.title/)
  assert.match(enSource, /title: 'Mission Control'/)
  assert.match(enSource, /activePlans: 'Active plans'/)
  assert.match(enSource, /agentRuns: 'Agent runs'/)
  assert.match(enSource, /cronJobs: 'Cron jobs'/)
  assert.match(homeSource, /setInterval/)
  assert.match(homeSource, /30_000/)
  assert.doesNotMatch(homeSource, /ChatPanel/)
})

test('Chat nav item is inactive on Home', () => {
  assert.doesNotMatch(navSource, /current === '\/console'/)
  assert.match(navSource, /current\.startsWith\('\/console\/chat'\)/)
})
