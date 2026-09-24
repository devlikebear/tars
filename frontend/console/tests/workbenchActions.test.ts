import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { buildWorkbenchActions } from '../src/lib/workbenchActions.ts'
import { chatCommandsEn } from '../src/i18n/sections/chatCommands.ts'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'

const chatSource = chatWorkbenchSource
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')

test('workbench action helper keeps active-plan operator surfaces one click away', () => {
  const actions = buildWorkbenchActions({
    sessionId: 'sess-1',
    hasPlan: true,
    activeTaskTitle: 'Wire validation',
  }, chatCommandsEn.workbench)

  assert.deepEqual(actions.map((action) => action.id), ['tasks', 'evidence', 'agentruntime', 'git'])
  assert.deepEqual(actions.map((action) => action.label), ['Tasks', 'Evidence', 'Agent Runtime', 'Git'])
  assert.equal(actions.find((action) => action.id === 'tasks')?.title, 'Open active plan tasks for Wire validation')
  assert.equal(actions.find((action) => action.id === 'evidence')?.title, 'Open plan evidence for Wire validation')
  assert.equal(actions.find((action) => action.id === 'tasks')?.panel, 'tasks')
  assert.equal(actions.find((action) => action.id === 'evidence')?.panel, 'tasks')
  assert.equal(actions.find((action) => action.id === 'evidence')?.tab, 'evidence')
  assert.equal(actions.find((action) => action.id === 'agentruntime')?.href, '/console/agentruntime')
  assert.equal(actions.find((action) => action.id === 'git')?.panel, 'git')
})

test('workbench action titles drop the task suffix when no task is active', () => {
  const actions = buildWorkbenchActions({ sessionId: 'sess-1', hasPlan: true, activeTaskTitle: '  ' }, chatCommandsEn.workbench)
  assert.equal(actions.find((action) => action.id === 'tasks')?.title, 'Open active plan tasks')
  assert.equal(actions.find((action) => action.id === 'evidence')?.title, 'Open plan evidence')
})

test('workbench action helper stays hidden without a selected session', () => {
  assert.deepEqual(buildWorkbenchActions({ sessionId: null, hasPlan: true }, chatCommandsEn.workbench), [])
})

test('Chat wires workbench actions to Tasks evidence, Agent Runtime, and Git', () => {
  assert.match(chatSource, /buildWorkbenchActions/)
  assert.match(chatSource, /workbench-action-strip/)
  assert.match(chatSource, /openEvidence/)
  assert.match(chatSource, /onNavigate\('\/console\/agentruntime'\)/)
  assert.match(chatSource, /openPanel\('git'\)/)
  assert.match(tasksPanelSource, /export function openEvidence/)
})
