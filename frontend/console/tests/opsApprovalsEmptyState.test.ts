import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')
const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('Ops explains the Approvals empty state workflow and cleanup trigger', () => {
  assert.match(source, /approvalGuideSteps/)
  assert.match(source, /approvalTriggerGuide/)
  assert.match(i18nEnSource, /emptyTitle: 'Approvals review queue'/)
  assert.match(i18nEnSource, /riskier workspace changes wait for your review/)
  assert.match(i18nEnSource, /New cleanup plan/)
  assert.match(i18nEnSource, /unused temporary files/)
  assert.match(i18nEnSource, /empty sessions/)
  assert.match(i18nEnSource, /Actual deletion waits for approval/)
  assert.match(i18nEnSource, /future Pulse signals/)
  assert.match(i18nEnSource, /candidate list/)
  assert.match(i18nEnSource, /Approve/)
  assert.match(i18nEnSource, /Reject/)
  assert.match(i18nEnSource, /result log/)
  assert.match(source, /class="approval-empty-guide/)
  assert.match(source, /title=\{\$t\.ops\.cleanupPlanTooltip\}/)
})
