import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const logsSource = readFileSync(new URL('../src/components/Logs.svelte', import.meta.url), 'utf8')
const routeComponentsSource = readFileSync(new URL('../src/lib/routeComponents.ts', import.meta.url), 'utf8')

test('/console/logs resolves to the global Logs page', () => {
  assert.deepEqual(resolveRoute('/console/logs'), { view: 'logs' })
  assert.match(routeComponentsSource, /logs:[^,]*import\('\.\.\/components\/Logs\.svelte'\)/)
  assert.match(appSource, /route\.view === 'logs'/)
  assert.match(appSource, /loadRouteComponent\('logs'\)/)
  assert.match(navSource, /id: 'logs'[\s\S]*path: '\/console\/logs'/)
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
