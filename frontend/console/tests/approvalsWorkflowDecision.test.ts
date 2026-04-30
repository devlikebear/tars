import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const decision = readFileSync(new URL('../../../docs/decisions/approvals-workflow.md', import.meta.url), 'utf8')

test('Approvals workflow RFC records the keep decision and routing policy', () => {
  assert.match(decision, /Decision: Keep Approvals/)
  assert.match(decision, /CON-041/)
  assert.match(decision, /manual cleanup plan/)
  assert.match(decision, /Pulse/)
  assert.match(decision, /autofix/)
  assert.match(decision, /approval queue/)
  assert.match(decision, /Do not remove/)
})
