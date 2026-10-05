import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'
import { navGroups } from '../src/lib/navGroups.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/components/Home.svelte', import.meta.url), 'utf8')
const navItem = (view: string) => navGroups.flatMap((g) => g.items).find((i) => i.view === view)
const enSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('/console resolves to the session board, the overview moves to /console/system (#971)', () => {
  assert.deepEqual(resolveRoute('/console'), { view: 'board' })
  assert.deepEqual(resolveRoute('/console/'), { view: 'board' })
  assert.deepEqual(resolveRoute('/console/system'), { view: 'home' })
  assert.deepEqual(resolveRoute('/console/nowhere'), { view: 'board' })
  assert.deepEqual(resolveRoute('/console/chat'), { view: 'chat' })
  assert.deepEqual(resolveRoute('/console/chat/session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.deepEqual(resolveRoute('/console/chat?session=session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.doesNotMatch(readFileSync(new URL('../src/lib/router.ts', import.meta.url), 'utf8'), /http:\/\/tars\.local/)
})

test('App renders the session board for the console entry route and Home for the overview', () => {
  assert.match(appSource, /import SessionBoard from '\.\/components\/SessionBoard\.svelte'/)
  assert.match(appSource, /import Home from '\.\/components\/Home\.svelte'/)
  assert.match(appSource, /let route = \$state<Route>\(\{ view: 'board' \}\)/)
  assert.match(appSource, /route\.view === 'board'/)
  assert.match(appSource, /route\.view === 'home'/)
  assert.match(appSource, /<Home onNavigate=\{navigate\} \/>/)
  assert.doesNotMatch(appSource, /import Chat from '\.\/components\/Chat\.svelte'/)
})

test('Home dashboard exposes system status, notifications, actions, and delivery', () => {
  assert.match(homeSource, /getServerStatus/)
  assert.match(homeSource, /getOpsStatus/)
  assert.match(homeSource, /getGlobalPlans/)
  assert.match(homeSource, /listAgentRuntimeRuns/)
  assert.match(homeSource, /listCronJobs/)
  assert.match(homeSource, /listSessions/)
  assert.match(homeSource, /getSyspromptFile/)
  assert.match(homeSource, /getConfig/)
  assert.doesNotMatch(homeSource, /getSessionTasks|loadContinueSession|continueSession|recentPlans|recentAgentRuns|recentMainSessions/)
  assert.match(homeSource, /\$t\.home\.title/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.activePlans/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.agentRuns/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.cronJobs/)
  assert.match(homeSource, /\$t\.home\.delivery\.title/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.activeSessions/)
  assert.match(homeSource, /\$t\.home\.statusStrip\.diskPressure/)
  assert.match(homeSource, /\$t\.home\.notifications\.title/)
  assert.match(homeSource, /\$t\.home\.recommendations\.title/)
  assert.doesNotMatch(homeSource, /\$t\.home\.(plans|agentRuns|cron|sessions|continue)\./)
  assert.doesNotMatch(homeSource, /plans-section|sessions-section|continue-section/)
  assert.match(enSource, /title: 'Mission Control'/)
  assert.match(enSource, /activePlans: 'Active plans'/)
  assert.match(enSource, /agentRuns: 'Agent runs'/)
  assert.match(enSource, /cronJobs: 'Cron jobs'/)
  assert.match(homeSource, /setInterval/)
  assert.match(homeSource, /30_000/)
  assert.doesNotMatch(homeSource, /ChatPanel/)
})

test('Chat nav item is inactive on the board and the overview', () => {
  // Nav highlights the item whose view matches the resolved route.
  assert.equal(resolveRoute('/console').view, 'board')
  assert.equal(navItem('board')?.path, '/console')
  assert.equal(navItem('home')?.path, '/console/system')
  assert.notEqual(navItem('chat')?.view, resolveRoute('/console').view)
  assert.equal(resolveRoute(navItem('chat')?.path ?? '').view, 'chat')
})
