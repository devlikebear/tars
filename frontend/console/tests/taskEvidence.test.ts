import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { tasksPanelEn } from '../src/i18n/sections/tasksPanel.ts'

const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/normalize.ts', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')

test('task evidence is typed, normalized, and rendered in task surfaces', () => {
  assert.match(typesSource, /TaskEvidence/)
  assert.match(typesSource, /evidence\?: TaskEvidence\[\]/)
  assert.match(apiSource, /normalizeTaskEvidence/)
  assert.match(apiSource, /evidence: Array\.isArray/)
  assert.match(tasksPanelSource, /Evidence/)
  assert.match(tasksPanelSource, /evidence_add/)
  assert.match(tasksPanelSource, /evidence_remove/)
  assert.match(tasksPanelSource, /task-evidence-list/)
  // ContractPanel was folded into TasksPanel as a tab; the Contract surface
  // still owns its own evidence list under the new layout.
  assert.match(tasksPanelSource, /TaskContract/)
  assert.match(tasksPanelSource, /contract/)
  assert.match(typesSource, /proof_state\?:/)
  assert.match(typesSource, /proof_origin\?:/)
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.proof\.independentlyVerified/)
  assert.equal(tasksPanelEn.proof.independentlyVerified, 'Independently verified')
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.proof\.reportedOnly/)
  assert.equal(tasksPanelEn.proof.reportedOnly, 'Reported only')
})
