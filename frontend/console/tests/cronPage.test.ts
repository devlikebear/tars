import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const cronSource = readFileSync(new URL('../src/components/Cron.svelte', import.meta.url), 'utf8')

test('/console/cron resolves to the global Cron page', () => {
  assert.deepEqual(resolveRoute('/console/cron'), { view: 'cron' })
  assert.match(appSource, /import Cron from '\.\/components\/Cron\.svelte'/)
  assert.match(appSource, /route\.view === 'cron'/)
  assert.match(appSource, /<Cron \/>/)
  assert.match(navSource, /id: 'cron'[\s\S]*path: '\/console\/cron'/)
})

test('Cron page manages global jobs with existing cron APIs', () => {
  assert.match(cronSource, /listCronJobs/)
  assert.match(cronSource, /createCronJob/)
  assert.match(cronSource, /updateCronJob/)
  assert.match(cronSource, /runCronJob/)
  assert.match(cronSource, /deleteCronJob/)
  assert.match(cronSource, /listCronRuns/)
  assert.match(cronSource, /deliveryTarget/)
  assert.match(cronSource, /handleToggleJob/)
  assert.match(cronSource, /expandedJob/)
  assert.match(cronSource, /run-history/)
})
