import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'
import { tasksPanelEn } from '../src/i18n/sections/tasksPanel.ts'

const chatSource = chatWorkbenchSource
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/tasks.ts', import.meta.url), 'utf8')
const apiNormalizeSource = readFileSync(new URL('../src/lib/api/normalize.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Chat mounts the task contract inside the tasks dock panel', () => {
  assert.match(chatSource, /TasksPanel/)
  assert.match(tasksPanelSource, /type TabId = 'tasks' \| 'contract' \| 'evidence'/)
  assert.match(chatSource, /togglePanel\('tasks'\)/)
})

test('Tasks panel edits, approves, and verifies the active session contract', () => {
  assert.match(typesSource, /TaskContract/)
  assert.match(typesSource, /contract\?: TaskContract/)
  assert.match(apiNormalizeSource, /contract: normalizeTaskContract/)
  assert.match(apiSource, /runTaskVerification/)
  assert.match(tasksPanelSource, /getSessionTasks/)
  assert.match(tasksPanelSource, /executeTasksAction/)
  assert.match(tasksPanelSource, /runTaskVerification/)
  assert.match(tasksPanelSource, /contract_update/)
  assert.match(tasksPanelSource, /contract_approve/)
  assert.match(typesSource, /proof_policy\?: TaskProofPolicy/)
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.contract\.requireProof/)
  assert.equal(tasksPanelEn.contract.requireProof, 'Require independent deterministic proof')
  assert.match(tasksPanelSource, /failure_state: contractProofFailureState/)
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.contract\.doneCriteria/)
  assert.equal(tasksPanelEn.contract.doneCriteria, 'Done criteria (one per line)')
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.contract\.verificationCommands/)
  assert.equal(tasksPanelEn.contract.verificationCommands, 'Verification commands (one per line)')
  assert.match(tasksPanelSource, /\$t\.tasksPanel\.contract\.runVerification/)
  assert.equal(tasksPanelEn.contract.runVerification, 'Run Verification')
})
