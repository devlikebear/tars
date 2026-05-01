import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { planProgressPercent, summarizeTasks } from '../src/lib/tasks.ts'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')

test('task progress helper counts completed tasks over total tasks', () => {
  assert.deepEqual(summarizeTasks([]), {
    total: 0,
    pending: 0,
    in_progress: 0,
    completed: 0,
    cancelled: 0,
  })
  assert.equal(planProgressPercent(summarizeTasks([])), 0)
  assert.equal(planProgressPercent(summarizeTasks([
    { id: '1', title: 'done', status: 'completed' },
    { id: '2', title: 'active', status: 'in_progress' },
    { id: '3', title: 'next', status: 'pending' },
  ])), 33)
})

test('Chat renders a clickable plan progress strip above the transcript', () => {
  assert.match(chatSource, /plan-progress-strip/)
  assert.match(chatSource, /tasksSummary\.plan_goal/)
  assert.match(chatSource, /planStripProgress/)
  assert.match(chatSource, /togglePanel\('tasks'\)/)
  assert.match(tasksPanelSource, /planProgressPercent/)
})
