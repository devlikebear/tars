import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { filterCommands, pageCommands, pageEntries, scoreCommand, type PageView, type PaletteCommand } from '../src/lib/commands.ts'

const routerSource = readFileSync(new URL('../src/lib/router.ts', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')

const titles = Object.fromEntries(pageEntries.map((p) => [p.view, p.view])) as Record<PageView, string>

function command(title: string, extra: Partial<PaletteCommand> = {}): PaletteCommand {
  return { id: title, group: 'page', title, run: () => {}, ...extra }
}

test('every routable view has exactly one palette entry', () => {
  const views = [...routerSource.matchAll(/\{ view: '([a-z-]+)'/g)].map((m) => m[1])
  const routeViews = new Set(views)
  const paletteViews = pageEntries.map((p) => p.view)
  assert.deepEqual(new Set(paletteViews), routeViews)
  assert.equal(paletteViews.length, routeViews.size, 'no duplicates')
})

test('admin-only palette entries match the views App.svelte gates by role', () => {
  const gated = new Set([...appSource.matchAll(/route\.view === '([a-z-]+)' && authRole !== 'user'/g)].map((m) => m[1]))
  const adminOnly = new Set(pageEntries.filter((p) => p.adminOnly && p.view !== 'onboarding').map((p) => p.view))
  assert.deepEqual(adminOnly, gated)
})

test('page commands hide admin pages from the user role and navigate on run', () => {
  const visited: string[] = []
  const admin = pageCommands(titles, 'admin', (path) => visited.push(path))
  const user = pageCommands(titles, 'user', () => {})
  assert.equal(admin.length, pageEntries.length)
  assert.ok(!user.some((c) => c.id === 'page:ops'))
  assert.ok(user.some((c) => c.id === 'page:memory'))
  admin.find((c) => c.id === 'page:pulse')?.run()
  assert.deepEqual(visited, ['/console/pulse'])
})

// Titles that share a prefix (Chat / Channels) need a few letters to split.
test('every page is the top result within four typed characters', () => {
  const english: Record<PageView, string> = {
    home: 'Home', chat: 'Chat', 'session-lineage': 'Lineage', tasks: 'Plans', agentruntime: 'Agent Runtime',
    memory: 'Memory', sysprompt: 'System Prompt', ops: 'Approvals', cron: 'Cron', logs: 'Logs',
    analytics: 'Analytics', config: 'Settings', extensions: 'Extensions', pulse: 'Pulse',
    reflection: 'Reflection', channels: 'Channels', onboarding: 'Setup Wizard',
  }
  const commands = pageCommands(english, 'admin', () => {})
  for (const cmd of commands) {
    // Shortest title prefix that makes this page the top hit.
    let reached = false
    for (let n = 1; n <= 4 && !reached; n++) {
      const top = filterCommands(commands, cmd.title.slice(0, n))[0]
      reached = top?.id === cmd.id
    }
    assert.ok(reached, `${cmd.title} should be the top result within 4 typed characters`)
  }
})

test('scoring prefers prefix, then word prefix, substring, and subsequence', () => {
  const q = 'mem'
  const prefix = scoreCommand(command('Memory'), q)
  const word = scoreCommand(command('Session memory'), q)
  const sub = scoreCommand(command('Remember'), q)
  const seq = scoreCommand(command('Message map'), q)
  assert.ok(prefix > word && word > sub && sub > seq && seq > 0, `${prefix} ${word} ${sub} ${seq}`)
  assert.equal(scoreCommand(command('Logs'), 'xyz'), 0)
})

test('keywords match with less weight than the title', () => {
  const byTitle = command('Settings')
  const byKeyword = command('Extensions', { keywords: ['settings'] })
  const ranked = filterCommands([byKeyword, byTitle], 'settings')
  assert.deepEqual(ranked.map((c) => c.title), ['Settings', 'Extensions'])
})

test('results keep each group together, best group first', () => {
  const ranked = filterCommands([
    command('Settings', { group: 'page' }),
    command('New session', { group: 'action' }),
    command('Setup wizard', { group: 'page' }),
    command('Session memory', { group: 'panel' }),
  ], 'se')
  // Page and panel tie on best score (a prefix match), so group order decides.
  assert.deepEqual(ranked.map((c) => c.group), ['page', 'page', 'panel', 'action'])
  assert.equal(ranked[0].title, 'Settings')
  // No group appears in two separate runs.
  const runs = ranked.map((c) => c.group).filter((g, i, all) => i === 0 || all[i - 1] !== g)
  assert.equal(new Set(runs).size, runs.length)
})

test('an empty query keeps group order, then source order', () => {
  const ranked = filterCommands([
    command('b-session', { group: 'session' }),
    command('a-page', { group: 'page' }),
    command('z-action', { group: 'action' }),
    command('c-page', { group: 'page' }),
  ], '')
  assert.deepEqual(ranked.map((c) => c.title), ['z-action', 'a-page', 'c-page', 'b-session'])
})
