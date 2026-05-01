import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const contractPanelSource = readFileSync(new URL('../src/components/ContractPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Chat mounts an editable task contract dock panel', () => {
  assert.match(chatSource, /ContractPanel/)
  assert.match(chatSource, /id: 'contract'/)
  assert.match(chatSource, /togglePanel\('contract'\)/)
})

test('Contract panel edits and approves the active session contract', () => {
  assert.match(typesSource, /TaskContract/)
  assert.match(typesSource, /contract\?: TaskContract/)
  assert.match(apiSource, /contract: normalizeTaskContract/)
  assert.match(contractPanelSource, /getSessionTasks/)
  assert.match(contractPanelSource, /executeTasksAction/)
  assert.match(contractPanelSource, /contract_update/)
  assert.match(contractPanelSource, /contract_approve/)
  assert.match(contractPanelSource, /Done criteria/)
  assert.match(contractPanelSource, /Verification commands/)
})
