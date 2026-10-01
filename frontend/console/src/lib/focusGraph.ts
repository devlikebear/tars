// The pipeline graph (docs/decisions/focus-mode.md §9 P5): a read-only
// @xyflow/svelte picture of one focus pipeline. Stages run left to right,
// each looping stage carries a self-loop edge labelled with its iteration
// count, and the plan's tasks sit inside the build node.
//
// buildFocusGraph is pure: FocusGraph.svelte renders what it returns, and
// tests check it under Node without a DOM.
import type { Edge, Node } from '@xyflow/svelte'
import type { FocusTranslations } from '../i18n/sections/focus.ts'
import type { FocusPipeline, FocusStage, FocusStageId, FocusStageStatus } from './types.ts'

// The stages that loop, the ones with a loop limit (focuspipeline
// DefaultLimits): build (implement → test), review (review → verify → fix),
// and pr (PR review → verify → fix).
export const LOOP_STAGES: FocusStageId[] = ['build', 'review', 'pr']

export type FocusGraphNodeData = {
  kind: 'stage' | 'task'
  label: string
  status?: FocusStageStatus
  iteration?: number
  limit?: number
  current?: boolean
  [key: string]: unknown
}

export type FocusGraphNode = Node<FocusGraphNodeData>
export type FocusGraphEdge = Edge<{ kind: 'forward' | 'loop' }>

export type FocusGraph = { nodes: FocusGraphNode[]; edges: FocusGraphEdge[] }

export type FocusGraphLabels = { stages: FocusTranslations['stages'] }

// Layout, in flow units.
const stageWidth = 150
const buildWidth = 230
const stageHeight = 64
const gap = 70
const taskHeight = 32
const taskGap = 8
const taskTop = 56
const taskInset = 10

function stageId(id: FocusStageId): string {
  return `stage-${id}`
}

function loopLabel(stage: FocusStage): string | undefined {
  if (stage.iteration < 1) return undefined
  return stage.limit ? `↻${stage.iteration}/${stage.limit}` : `↻${stage.iteration}`
}

export function buildFocusGraph(p: FocusPipeline, labels: FocusGraphLabels): FocusGraph {
  const tasks = p.plan?.tasks ?? []
  const nodes: FocusGraphNode[] = []
  const edges: FocusGraphEdge[] = []
  let x = 0

  p.stages.forEach((stage, i) => {
    const isBuild = stage.id === 'build'
    const width = isBuild && tasks.length > 0 ? buildWidth : stageWidth
    const height = isBuild && tasks.length > 0 ? taskTop + tasks.length * (taskHeight + taskGap) + taskInset : stageHeight
    const current = stage.id === p.current
    nodes.push({
      id: stageId(stage.id),
      type: 'focusStage',
      position: { x, y: 0 },
      width,
      height,
      data: {
        kind: 'stage',
        label: labels.stages[stage.id] ?? stage.id,
        status: stage.status,
        iteration: stage.iteration,
        limit: stage.limit,
        current,
      },
      class: `focus-graph-stage focus-graph-${stage.status}${current ? ' focus-graph-current' : ''}`,
      draggable: false,
      selectable: false,
    })

    if (isBuild) {
      tasks.forEach((task, n) => {
        nodes.push({
          id: `task-${n}`,
          type: 'focusTask',
          parentId: stageId('build'),
          extent: 'parent',
          position: { x: taskInset, y: taskTop + n * (taskHeight + taskGap) },
          width: width - taskInset * 2,
          height: taskHeight,
          data: { kind: 'task', label: `${n + 1}. ${task.title}` },
          class: 'focus-graph-task',
          draggable: false,
          selectable: false,
        })
      })
    }

    if (i > 0) {
      const prev = p.stages[i - 1]
      const skipped = prev.status === 'skipped' || stage.status === 'skipped'
      edges.push({
        id: `forward-${prev.id}-${stage.id}`,
        source: stageId(prev.id),
        target: stageId(stage.id),
        sourceHandle: 'out',
        targetHandle: 'in',
        type: 'smoothstep',
        animated: current && stage.status === 'active',
        class: `focus-graph-edge${skipped ? ' focus-graph-edge-skipped' : ''}`,
        data: { kind: 'forward' },
      })
    }

    if (LOOP_STAGES.includes(stage.id) && stage.status !== 'skipped') {
      edges.push({
        id: `loop-${stage.id}`,
        source: stageId(stage.id),
        target: stageId(stage.id),
        sourceHandle: 'loop-out',
        targetHandle: 'loop-in',
        type: 'smoothstep',
        label: loopLabel(stage),
        animated: current && stage.status === 'active',
        class: `focus-graph-loop${stage.iteration > 1 ? ' focus-graph-loop-repeated' : ''}`,
        data: { kind: 'loop' },
      })
    }

    x += width + gap
  })

  return { nodes, edges }
}
