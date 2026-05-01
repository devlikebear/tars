import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')
const contractPanelSource = readFileSync(new URL('../src/components/ContractPanel.svelte', import.meta.url), 'utf8')

test('task evidence is typed, normalized, and rendered in task surfaces', () => {
  assert.match(typesSource, /TaskEvidence/)
  assert.match(typesSource, /evidence\?: TaskEvidence\[\]/)
  assert.match(apiSource, /normalizeTaskEvidence/)
  assert.match(apiSource, /evidence: Array\.isArray/)
  assert.match(tasksPanelSource, /Evidence/)
  assert.match(tasksPanelSource, /evidence_add/)
  assert.match(tasksPanelSource, /evidence_remove/)
  assert.match(tasksPanelSource, /task-evidence-list/)
  assert.match(contractPanelSource, /Evidence/)
  assert.match(contractPanelSource, /contractEvidence/)
})
