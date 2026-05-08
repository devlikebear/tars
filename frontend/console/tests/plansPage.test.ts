import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolveRoute } from '../src/lib/router.ts'
import { aggregatePlanStatusCount, filterPlansBySummaryCard } from '../src/lib/plans.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const routeComponentsSource = readFileSync(new URL('../src/lib/routeComponents.ts', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const enSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
const koSource = readFileSync(new URL('../src/i18n/ko.ts', import.meta.url), 'utf8')

test('/console/tasks resolves to the global Plans page', () => {
  assert.deepEqual(resolveRoute('/console/tasks'), { view: 'tasks' })
  assert.deepEqual(resolveRoute('/console/chat?session=session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.doesNotMatch(appSource, /import Plans from '\.\/components\/Plans\.svelte'/)
  assert.match(routeComponentsSource, /tasks: memoizeRouteLoader\(\(\) => import\('\.\.\/components\/Plans\.svelte'\)\)/)
  assert.match(appSource, /route\.view === 'tasks'/)
  assert.match(navSource, /id: 'plans'[\s\S]*path: '\/console\/tasks'/)
  assert.match(enSource, /plans:\s*'Plans'/)
  assert.match(koSource, /plans:\s*'계획'/)
})

test('Plans page wires the global active plans API and session navigation', () => {
  const plansSource = readFileSync(new URL('../src/components/Plans.svelte', import.meta.url), 'utf8')
  assert.match(typesSource, /GlobalPlanItem/)
  assert.match(typesSource, /GlobalPlansResponse/)
  assert.match(apiSource, /getGlobalPlans/)
  assert.match(apiSource, /\/v1\/admin\/tasks\?active=true/)
  assert.match(plansSource, /getGlobalPlans/)
  assert.match(plansSource, /progressPercent/)
  assert.match(plansSource, /\/console\/chat\?session=/)
  assert.match(plansSource, /\$t\.plans\.emptyTitle/)
})

test('Plans summary cards filter sessions by task status counts', () => {
  const items = [
    {
      session: { id: 'active' },
      tasks: [
        { id: 'a-1', title: 'running', status: 'in_progress' },
        { id: 'a-2', title: 'queued', status: 'pending' },
      ],
      summary: { total: 2, in_progress: 1, pending: 1, completed: 0 },
    },
    {
      session: { id: 'done' },
      tasks: [
        { id: 'd-1', title: 'finished', status: 'completed' },
      ],
      summary: { total: 1, in_progress: 0, pending: 0, completed: 1 },
    },
    {
      session: { id: 'fallback' },
      tasks: [
        { id: 'f-1', title: 'finished without summary', status: 'completed' },
      ],
      summary: {},
    },
  ]

  assert.equal(aggregatePlanStatusCount(items, 'completed'), 2)
  assert.deepEqual(filterPlansBySummaryCard(items, 'all').map((item) => item.session.id), ['active', 'done', 'fallback'])
  assert.deepEqual(filterPlansBySummaryCard(items, 'in_progress').map((item) => item.session.id), ['active'])
  assert.deepEqual(filterPlansBySummaryCard(items, 'pending').map((item) => item.session.id), ['active'])
  assert.deepEqual(filterPlansBySummaryCard(items, 'completed').map((item) => item.session.id), ['done', 'fallback'])
})

test('Plans page makes aggregate cards clickable status filters', () => {
  const plansSource = readFileSync(new URL('../src/components/Plans.svelte', import.meta.url), 'utf8')
  assert.match(plansSource, /activeSummaryFilter/)
  assert.match(plansSource, /filterPlansBySummaryCard/)
  assert.match(plansSource, /aggregatePlanStatusCount/)
  assert.match(plansSource, /onclick=\{\(\) => setSummaryFilter\(card\.filter\)\}/)
  assert.match(plansSource, /aria-pressed=\{activeSummaryFilter === card\.filter\}/)
  assert.match(plansSource, /\{#each filteredPlans as item/)
})
