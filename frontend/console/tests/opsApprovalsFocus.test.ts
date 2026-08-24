import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { en } from '../src/i18n/en.ts'

const opsSource = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/ops.ts', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const routerSource = readFileSync(new URL('../src/lib/router.ts', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')

test('Operations becomes an Approvals-focused page with legacy ops routing', () => {
  const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
  assert.match(opsSource, /<h2>\{\$t\.ops\.title\}<\/h2>/)
  assert.match(i18nEnSource, /title: 'Approvals'/)
  assert.match(i18nEnSource, /Review risky cleanup plans before TARS applies them/)
  assert.match(opsSource, /approval-empty-guide/)
  assert.match(i18nEnSource, /Automation Audit/)
  assert.match(opsSource, /listAutomationAudit\(25\)/)
  assert.match(apiSource, /\/v1\/ops\/automation-audit/)
  assert.doesNotMatch(opsSource, /getOpsStatus/)
  assert.doesNotMatch(opsSource, /listCronJobs/)
  assert.doesNotMatch(opsSource, /createCronJob/)

  assert.equal(en.nav.items.ops, 'Approvals')
  assert.match(navSource, /id: 'ops'[\s\S]*path: '\/console\/approvals'/)

  assert.match(routerSource, /\/approvals/)
  assert.match(routerSource, /\/ops/)
  assert.match(routerSource, /view: 'ops'/)

  assert.match(appSource, /loadRouteComponent\('ops'\)/)
})
