import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const opsSource = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const routerSource = readFileSync(new URL('../src/lib/router.ts', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')

test('Operations becomes an Approvals-focused page with legacy ops routing', () => {
  assert.match(opsSource, /<h2>Approvals<\/h2>/)
  assert.match(opsSource, /Review risky cleanup plans before TARS applies them/)
  assert.match(opsSource, /approval-empty-guide/)
  assert.doesNotMatch(opsSource, /getOpsStatus/)
  assert.doesNotMatch(opsSource, /listCronJobs/)
  assert.doesNotMatch(opsSource, /createCronJob/)
  assert.doesNotMatch(opsSource, /Cron jobs/)
  assert.doesNotMatch(opsSource, /System health/)
  assert.doesNotMatch(opsSource, /Processes/)
  assert.doesNotMatch(opsSource, /Free space/)

  assert.match(navSource, /label: 'Approvals'/)
  assert.match(navSource, /path: '\/console\/approvals'/)

  assert.match(routerSource, /\/approvals/)
  assert.match(routerSource, /\/ops/)
  assert.match(routerSource, /view: 'ops'/)

  assert.match(appSource, /<Ops \/>/)
})
