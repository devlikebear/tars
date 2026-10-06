import test from 'node:test'
import assert from 'node:assert/strict'

import { progressLine, stageKindOf, stageLabel, stepperItems, templateText, turnStage } from '../src/lib/focus.ts'
import { buildFocusGraph } from '../src/lib/focusGraph.ts'
import { focusEn, focusKo } from '../src/i18n/sections/focus.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as FocusStoreModule from '../src/lib/stores/focusStore.svelte.ts'
import type { FocusActionResult, FocusPipeline } from '../src/lib/types.ts'

const { FocusStore } = await compileSvelteModule<typeof FocusStoreModule>('src/lib/stores/focusStore.svelte.ts')

// A research pipeline as the server stores it: its own stage ids, each with
// the kind it runs as and the template's label.
function research(extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1,
    session_id: 's1',
    goal: 'why is the sky blue',
    template: 'research',
    current: 'research',
    stages: [
      { id: 'plan', status: 'done', iteration: 1, label: 'Scope' },
      { id: 'research', kind: 'build', status: 'active', iteration: 1, limit: 3, label: 'Research' },
      { id: 'report', kind: 'build', status: 'pending', iteration: 0, label: 'Report' },
      { id: 'check', kind: 'review', status: 'pending', iteration: 0, label: 'Check' },
    ],
    plan: {
      goal: 'g',
      tasks: [
        { title: 'find sources', done: 'notes.md', stage: 'research' },
        { title: 'write it', done: 'report.md', stage: 'report' },
        { title: 'anything else', done: '' },
        { title: 'lost', done: '', stage: 'check' },
      ],
      stages: ['plan', 'research', 'report', 'check'],
      verify: [],
    },
    cards: [],
    updated_at: '2026-10-06T00:00:00Z',
    ...extra,
  }
}

test('stageKindOf: a template stage runs as its kind, a development stage as itself', () => {
  const p = research()
  assert.equal(stageKindOf(p.stages, 'research'), 'build')
  assert.equal(stageKindOf(p.stages, 'check'), 'review')
  assert.equal(stageKindOf(p.stages, 'plan'), 'plan')
  assert.equal(stageKindOf(undefined, 'merge'), 'merge')
  assert.equal(stageKindOf(p.stages, 'nowhere'), null)
  assert.equal(stageKindOf(p.stages, null), null)
})

test('stageLabel: the console wording of a built-in template, else the template label, else the stage name, else the id', () => {
  const p = research()
  assert.equal(stageLabel(p, 'research', focusEn), 'Research')
  assert.equal(stageLabel(p, 'check', focusKo), '검증')
  assert.equal(stageLabel(p, 'plan', focusKo), '범위')
  // A workspace template is shown as written, in any language.
  const blog = research({ template: 'blog', stages: [{ id: 'plan', status: 'done', iteration: 1 }, { id: 'write', kind: 'build', status: 'active', iteration: 1, label: 'Write it' }] })
  assert.equal(stageLabel(blog, 'write', focusKo), 'Write it')
  assert.equal(stageLabel(blog, 'plan', focusKo), '계획')
  assert.equal(stageLabel(blog, 'unknown', focusKo), 'unknown')
  // Development pipelines keep their names.
  assert.equal(stageLabel({ stages: [{ id: 'build' }] }, 'build', focusKo), '구현')
  assert.equal(stageLabel(null, 'pr_review', focusEn), 'PR review')
})

test('templateText knows the built-in templates in both languages and nothing else', () => {
  assert.equal(templateText('writing', focusKo.templates)?.name, '글쓰기')
  assert.equal(templateText(undefined, focusEn.templates)?.name, 'Development')
  assert.equal(templateText('blog', focusEn.templates), null)
  assert.equal(templateText('writing', undefined), null)
  assert.deepEqual(Object.keys(focusKo.templates.research.stages), Object.keys(focusEn.templates.research.stages))
})

test('stepperItems names a template pipeline by its own stages', () => {
  const items = stepperItems(research(), focusKo.stages, focusKo.templates)
  assert.deepEqual(items.map((i) => i.id), ['plan', 'research', 'report', 'check'])
  assert.deepEqual(items.map((i) => i.label), ['범위', '조사', '보고서', '검증'])
  assert.equal(items[1].current, true)
  assert.deepEqual(stepperItems(research()).map((i) => i.label), ['Scope', 'Research', 'Report', 'Check'])
})

test('progressLine leads with a template stage name when given one', () => {
  assert.equal(progressLine([], { stage: 'build' }), 'Implementing')
  assert.equal(progressLine([], { stage: 'build', lead: 'Draft' }), 'Draft')
  assert.equal(progressLine([{ type: 'file_change', path: 'ch1.md' }], { lead: '초고', text: focusKo.progress }), '초고 · 파일 1개 변경')
})

test('turnStage reads template stage ids, digits included', () => {
  assert.equal(turnStage('go\n\n<focus-stage>\nFocus mode — current stage: draft2 (iteration 1 of 3).\n</focus-stage>'), 'draft2')
  assert.equal(turnStage('go\n\n<focus-stage>\nFocus mode — current stage: pr_review.\n</focus-stage>'), 'pr_review')
})

test('the graph puts each task in its work stage and loops template stages by kind', () => {
  const graph = buildFocusGraph(research(), { stages: focusEn.stages, templates: focusEn.templates })
  const stageNodes = graph.nodes.filter((n) => n.data.kind === 'stage')
  assert.deepEqual(stageNodes.map((n) => n.data.label), ['Scope', 'Research', 'Report', 'Check'])
  const parents = Object.fromEntries(graph.nodes.filter((n) => n.data.kind === 'task').map((n) => [n.id, n.parentId]))
  // A task naming no work stage (or a stage that is not one) sits in the first.
  assert.deepEqual(parents, { 'task-0': 'stage-research', 'task-1': 'stage-report', 'task-2': 'stage-research', 'task-3': 'stage-research' })
  assert.deepEqual(graph.nodes.filter((n) => n.data.kind === 'task').map((n) => n.data.label), ['1. find sources', '3. anything else', '4. lost', '2. write it'])
  const loops = graph.edges.filter((e) => e.data?.kind === 'loop').map((e) => e.id)
  assert.deepEqual(loops, ['loop-research', 'loop-report', 'loop-check'])
  const researchNode = stageNodes[1]
  const reportNode = stageNodes[2]
  assert.ok((researchNode.height ?? 0) > (reportNode.height ?? 0), 'three tasks need more room than one')
})

test('setGoal turns goal mode on and off through the store', async () => {
  const off = research()
  const on = research({ goal_mode: { enabled: true, pushes: 0, max_pushes: 20 } })
  const calls: unknown[][] = []
  const api = {
    getPipeline: async () => off,
    getSession: async (id: string) => ({ id, title: 'Task', current_dir: 'repo', created_at: '', updated_at: '' }),
    getHistory: async () => [],
    listCheckpoints: async (id: string) => ({ session_id: id, turns: [] }),
    getCheckpointDiff: async () => ({ turn_id: '', scope: 'turn', root: '', from: '', to: '', files: [] }),
    streamChat: async () => {},
    attachChatStream: async () => false,
    goal: async (...args: unknown[]): Promise<FocusActionResult> => {
      calls.push(args)
      return { pipeline: args[1] ? on : { ...off, goal_mode: { enabled: false, pushes: 0, max_pushes: 20, end_reason: 'disabled' } }, next_prompt: '' }
    },
  }
  const store = new FocusStore(api as never, null)
  assert.equal(await store.setGoal(true), false, 'no session loaded yet')
  await store.load('s1')
  assert.equal(await store.setGoal(true), true)
  assert.equal(store.pipeline?.goal_mode?.enabled, true)
  assert.equal(await store.setGoal(false), true)
  assert.equal(store.pipeline?.goal_mode?.end_reason, 'disabled')
  assert.deepEqual(calls, [['s1', true], ['s1', false]])
  // The stage phrase of a template stage comes from its kind.
  assert.equal(store.progress('research'), 'Implementing')
  assert.equal(store.progress('research', undefined, 'Research'), 'Research')

  const without = new FocusStore({ ...api, goal: undefined } as never, null)
  await without.load('s1')
  assert.equal(await without.setGoal(true), false, 'an API without the goal route')
  store.dispose()
  without.dispose()
})
