import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const routeComponentsSource = readFileSync(new URL('../src/lib/routeComponents.ts', import.meta.url), 'utf8')
const viteConfigSource = readFileSync(new URL('../vite.config.ts', import.meta.url), 'utf8')

const lazyRouteComponents = [
  'Chat',
  'SessionLineageGraph',
  'Plans',
  'MemoryCenter',
  'SyspromptCenter',
  'Ops',
  'Cron',
  'Logs',
  'Analytics',
  'Config',
  'Extensions',
  'Pulse',
  'Reflection',
  'Channels',
  'AgentRuntimeRunView',
]

test('heavy console routes are loaded through dynamic route component imports', () => {
  for (const component of lazyRouteComponents) {
    assert.doesNotMatch(
      appSource,
      new RegExp(`import ${component} from '\\./components/${component}\\.svelte'`),
      `${component} should not be statically imported by App.svelte`,
    )
    assert.match(
      routeComponentsSource,
      new RegExp(`import\\('\\.\\./components/${component}\\.svelte'\\)`),
      `${component} should be registered as a dynamic route import`,
    )
  }
})

test('first-paint shell routes stay eager while page routes are lazy', () => {
  assert.match(appSource, /import Shell from '\.\/components\/Shell\.svelte'/)
  assert.match(appSource, /import Home from '\.\/components\/Home\.svelte'/)
  assert.match(appSource, /import Onboarding from '\.\/components\/Onboarding\.svelte'/)
  assert.match(appSource, /import Login from '\.\/components\/Login\.svelte'/)
  assert.match(routeComponentsSource, /memoizeRouteLoader/)
  assert.match(routeComponentsSource, /export function loadRouteComponent/)
})

test('production chunk warning budget stays close to the Vite default', () => {
  assert.match(viteConfigSource, /chunkSizeWarningLimit:\s*550/)
})
