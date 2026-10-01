import test from 'node:test'
import assert from 'node:assert/strict'

import { buildFocusGraph, LOOP_STAGES } from '../src/lib/focusGraph.ts'
import { focusEn } from '../src/i18n/sections/focus.ts'
import type { FocusPipeline, FocusStage } from '../src/lib/types.ts'

function stages(over: Partial<Record<FocusStage['id'], Partial<FocusStage>>> = {}): FocusStage[] {
  return (['plan', 'build', 'review', 'pr', 'pr_review', 'merge'] as const).map((id) => ({
    id,
    status: 'pending',
    iteration: 0,
    ...over[id],
  }))
}

function pipeline(extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1,
    session_id: 's1',
    goal: 'g',
    stages: stages({ plan: { status: 'done', iteration: 1 }, build: { status: 'active', iteration: 2, limit: 3 } }),
    current: 'build',
    cards: [],
    updated_at: '2026-10-01T00:00:00Z',
    ...extra,
  }
}

const labels = { stages: focusEn.stages }

test('stage nodes come in pipeline order with their status, left to right', () => {
  const { nodes } = buildFocusGraph(pipeline(), labels)
  const stageNodes = nodes.filter((n) => n.data.kind === 'stage')
  assert.deepEqual(stageNodes.map((n) => n.id), ['stage-plan', 'stage-build', 'stage-review', 'stage-pr', 'stage-pr_review', 'stage-merge'])
  assert.deepEqual(stageNodes.map((n) => n.data.status), ['done', 'active', 'pending', 'pending', 'pending', 'pending'])
  assert.equal(stageNodes[0].data.label, focusEn.stages.plan)
  assert.equal(stageNodes.find((n) => n.data.current)?.id, 'stage-build')
  for (let i = 1; i < stageNodes.length; i++) {
    assert.ok(stageNodes[i].position.x > stageNodes[i - 1].position.x, 'left to right')
  }
  assert.ok(nodes.every((n) => n.draggable === false), 'read-only')
})

test('forward edges join consecutive stages; skipped stages and their edges are marked', () => {
  const p = pipeline({ stages: stages({ plan: { status: 'done', iteration: 1 }, build: { status: 'active', iteration: 1 }, review: { status: 'skipped' }, pr_review: { status: 'skipped' } }) })
  const { nodes, edges } = buildFocusGraph(p, labels)
  const forward = edges.filter((e) => e.data?.kind === 'forward')
  assert.deepEqual(forward.map((e) => `${e.source}>${e.target}`), [
    'stage-plan>stage-build',
    'stage-build>stage-review',
    'stage-review>stage-pr',
    'stage-pr>stage-pr_review',
    'stage-pr_review>stage-merge',
  ])
  assert.ok(forward.every((e) => e.sourceHandle === 'out' && e.targetHandle === 'in'))
  assert.equal(nodes.find((n) => n.id === 'stage-review')?.data.status, 'skipped')
  assert.match(String(nodes.find((n) => n.id === 'stage-review')?.class), /focus-graph-skipped/)
  assert.match(String(forward[1].class), /focus-graph-edge-skipped/)
  assert.doesNotMatch(String(forward[0].class), /skipped/)
})

test('loop edges sit on the looping stages and carry the iteration count', () => {
  assert.deepEqual(LOOP_STAGES, ['build', 'review', 'pr'])
  const p = pipeline({
    stages: stages({
      plan: { status: 'done', iteration: 1 },
      build: { status: 'done', iteration: 3, limit: 3 },
      review: { status: 'active', iteration: 2 },
      pr: { status: 'skipped' },
    }),
    current: 'review',
  })
  const loops = buildFocusGraph(p, labels).edges.filter((e) => e.data?.kind === 'loop')
  // A skipped stage has no loop.
  assert.deepEqual(loops.map((e) => e.source), ['stage-build', 'stage-review'])
  for (const e of loops) {
    assert.equal(e.source, e.target, 'a loop returns to its stage')
    assert.equal(e.sourceHandle, 'loop-out')
    assert.equal(e.targetHandle, 'loop-in')
  }
  assert.equal(loops[0].label, '↻3/3')
  assert.equal(loops[1].label, '↻2')
  assert.equal(loops[1].animated, true, 'the current stage loops live')
  assert.equal(loops[0].animated, false)
})

test('a stage that has not run yet shows its loop without a count', () => {
  const loops = buildFocusGraph(pipeline(), labels).edges.filter((e) => e.data?.kind === 'loop')
  const review = loops.find((e) => e.source === 'stage-review')
  assert.ok(review)
  assert.equal(review.label, undefined)
})

test('plan tasks are child nodes inside the build node, in plan order', () => {
  const p = pipeline({
    plan: {
      goal: 'g',
      tasks: [
        { title: 'Server listing', done: 'tests pass' },
        { title: 'Console view', done: 'check green' },
      ],
      stages: ['plan', 'build', 'review'],
      verify: ['make test'],
    },
  })
  const { nodes } = buildFocusGraph(p, labels)
  const build = nodes.find((n) => n.id === 'stage-build')
  const tasks = nodes.filter((n) => n.data.kind === 'task')
  assert.deepEqual(tasks.map((n) => n.data.label), ['1. Server listing', '2. Console view'])
  assert.ok(tasks.every((n) => n.parentId === 'stage-build' && n.extent === 'parent'))
  assert.ok(nodes.indexOf(build!) < nodes.indexOf(tasks[0]), 'a parent comes before its children')
  assert.ok(tasks[1].position.y > tasks[0].position.y)
  // The build node grows to hold its tasks.
  const plain = buildFocusGraph(pipeline(), labels).nodes.find((n) => n.id === 'stage-build')
  assert.ok((build?.height ?? 0) > (plain?.height ?? 0))
  assert.ok((tasks[1].position.y + (tasks[1].height ?? 0)) <= (build?.height ?? 0), 'tasks fit inside')
})

test('a pipeline without a plan has no task nodes', () => {
  const { nodes } = buildFocusGraph(pipeline(), labels)
  assert.equal(nodes.filter((n) => n.data.kind === 'task').length, 0)
})
