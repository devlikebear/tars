import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const enSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
const koSource = readFileSync(new URL('../src/i18n/ko.ts', import.meta.url), 'utf8')

test('/console/tasks resolves to the global Plans page', () => {
  assert.deepEqual(resolveRoute('/console/tasks'), { view: 'tasks' })
  assert.deepEqual(resolveRoute('/console/chat?session=session-1'), { view: 'chat', sessionId: 'session-1' })
  assert.match(appSource, /import Plans from '\.\/components\/Plans\.svelte'/)
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
  assert.match(plansSource, /No active plans/)
})
