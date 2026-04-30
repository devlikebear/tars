import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')

test('Ops explains the Approvals empty state workflow and cleanup trigger', () => {
  assert.match(source, /approvalGuideSteps/)
  assert.match(source, /approvalTriggerGuide/)
  assert.match(source, /Approvals review queue/)
  assert.match(source, /riskier workspace changes wait for your review/)
  assert.match(source, /New cleanup plan/)
  assert.match(source, /unused temporary files/)
  assert.match(source, /empty sessions/)
  assert.match(source, /Actual deletion waits for approval/)
  assert.match(source, /future Pulse signals/)
  assert.match(source, /candidate list/)
  assert.match(source, /Approve/)
  assert.match(source, /Reject/)
  assert.match(source, /result log/)
  assert.match(source, /class="approval-empty-guide/)
  assert.match(source, /title={cleanupPlanTooltip}/)
})
