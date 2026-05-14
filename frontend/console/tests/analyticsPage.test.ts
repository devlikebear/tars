import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const analyticsSource = readFileSync(new URL('../src/components/Analytics.svelte', import.meta.url), 'utf8')
const routeComponentsSource = readFileSync(new URL('../src/lib/routeComponents.ts', import.meta.url), 'utf8')
const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('/console/analytics resolves to the Analytics page', () => {
  assert.deepEqual(resolveRoute('/console/analytics'), { view: 'analytics' })
  assert.match(routeComponentsSource, /analytics:[^,]*import\('\.\.\/components\/Analytics\.svelte'\)/)
  assert.match(appSource, /route\.view === 'analytics'/)
  assert.match(appSource, /loadRouteComponent\('analytics'\)/)
  assert.match(navSource, /id: 'analytics'[\s\S]*path: '\/console\/analytics'/)
})

test('Analytics page renders usage charts and tables', () => {
  assert.match(apiSource, /getAnalytics/)
  assert.match(apiSource, /\/v1\/admin\/analytics/)
  assert.match(analyticsSource, /selectedDays/)
  assert.match(analyticsSource, /7/)
  assert.match(analyticsSource, /30/)
  assert.match(analyticsSource, /90/)
  assert.match(analyticsSource, /<svg/)
  assert.match(analyticsSource, /input-bar/)
  assert.match(analyticsSource, /output-bar/)
  assert.match(analyticsSource, /models/)
  assert.match(analyticsSource, /skills/)
  assert.match(i18nEnSource, /emptyTitle: 'No usage yet'/)
})
