import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Chat mounts the task contract inside the tasks dock panel', () => {
  assert.match(chatSource, /TasksPanel/)
  assert.match(tasksPanelSource, /type TabId = 'tasks' \| 'contract' \| 'evidence'/)
  assert.match(chatSource, /togglePanel\('tasks'\)/)
})

test('Tasks panel edits, approves, and verifies the active session contract', () => {
  assert.match(typesSource, /TaskContract/)
  assert.match(typesSource, /contract\?: TaskContract/)
  assert.match(apiSource, /contract: normalizeTaskContract/)
  assert.match(apiSource, /runTaskVerification/)
  assert.match(tasksPanelSource, /getSessionTasks/)
  assert.match(tasksPanelSource, /executeTasksAction/)
  assert.match(tasksPanelSource, /runTaskVerification/)
  assert.match(tasksPanelSource, /contract_update/)
  assert.match(tasksPanelSource, /contract_approve/)
  assert.match(tasksPanelSource, /Done criteria/)
  assert.match(tasksPanelSource, /Verification commands/)
  assert.match(tasksPanelSource, /Run Verification/)
})
