import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'
import { navGroups } from '../src/lib/navGroups.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/system.ts', import.meta.url), 'utf8')
const navItem = (view: string) => navGroups.flatMap((g) => g.items).find((i) => i.view === view)
const logsSource = readFileSync(new URL('../src/components/Logs.svelte', import.meta.url), 'utf8')
const routeComponentsSource = readFileSync(new URL('../src/lib/routeComponents.ts', import.meta.url), 'utf8')

test('/console/logs resolves to the global Logs page', () => {
  assert.deepEqual(resolveRoute('/console/logs'), { view: 'logs' })
  assert.match(routeComponentsSource, /logs:[^,]*import\('\.\.\/components\/Logs\.svelte'\)/)
  assert.match(appSource, /route\.view === 'logs'/)
  assert.match(appSource, /loadRouteComponent\('logs'\)/)
  assert.equal(navItem('logs')?.path, '/console/logs')
})

test('Logs page exposes filtering and refresh controls', () => {
  assert.match(apiSource, /getLogs/)
  assert.match(apiSource, /\/v1\/admin\/logs/)
  assert.match(logsSource, /selectedFile/)
  assert.match(logsSource, /selectedLevel/)
  assert.match(logsSource, /selectedComponent/)
  assert.match(logsSource, /lineCount/)
  assert.match(logsSource, /autoRefresh/)
  assert.match(logsSource, /level-error/)
  assert.match(logsSource, /level-warn/)
  assert.match(logsSource, /level-debug/)
})
