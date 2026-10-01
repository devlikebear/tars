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

// width and height are the content's size in flow units, for sizing the
// canvas.
export type FocusGraph = { nodes: FocusGraphNode[]; edges: FocusGraphEdge[]; width: number; height: number }

export type FocusGraphLabels = { stages: FocusTranslations['stages'] }

export type FocusGraphOptions = {
  // Stages per row; the rest wrap to rows below. Default: all in one row.
  columns?: number
}

// Readability: the canvas never zooms below GRAPH_MIN_ZOOM, and node text is
// GRAPH_NODE_FONT_PX, so text is at least 11px on screen. GRAPH_PADDING is
// fitView's padding (a fraction of the content).
export const GRAPH_MIN_ZOOM = 0.85
export const GRAPH_NODE_FONT_PX = 13
export const GRAPH_PADDING = 0.04

// Layout, in flow units.
const stageWidth = 140
const buildWidth = 220
const stageHeight = 64
const gap = 48
// Room under each row for its loop edges and their labels.
const loopSpace = 44
const rowGap = loopSpace + 24
// Under the last row: its loops and, at the left, the zoom controls.
const bottomSpace = loopSpace + 24
const taskHeight = 32
const taskGap = 8
const taskTop = 60
const taskInset = 10

function stageId(id: FocusStageId): string {
  return `stage-${id}`
}

function loopLabel(stage: FocusStage): string | undefined {
  if (stage.iteration < 1) return undefined
  return stage.limit ? `↻${stage.iteration}/${stage.limit}` : `↻${stage.iteration}`
}

export function buildFocusGraph(p: FocusPipeline, labels: FocusGraphLabels, options: FocusGraphOptions = {}): FocusGraph {
  const tasks = p.plan?.tasks ?? []
  const columns = Math.max(1, Math.min(options.columns ?? p.stages.length, p.stages.length))
  const nodes: FocusGraphNode[] = []
  const edges: FocusGraphEdge[] = []
  const sizeOf = (stage: FocusStage) => {
    const holdsTasks = stage.id === 'build' && tasks.length > 0
    return {
      width: holdsTasks ? buildWidth : stageWidth,
      height: holdsTasks ? taskTop + tasks.length * (taskHeight + taskGap) + taskInset : stageHeight,
    }
  }
  // Each row is as tall as its tallest stage.
  const rowHeights: number[] = []
  p.stages.forEach((stage, i) => {
    const row = Math.floor(i / columns)
    rowHeights[row] = Math.max(rowHeights[row] ?? 0, sizeOf(stage).height)
  })
  const rowTop = (row: number) => rowHeights.slice(0, row).reduce((sum, h) => sum + h + rowGap, 0)
  // Rows snake: even rows run left to right, odd rows right to left, so a
  // row's first stage sits under the previous row's last one and the wrap
  // edge is a short drop. Columns line up across rows, each as wide as its
  // widest stage.
  const rowOf = (i: number) => Math.floor(i / columns)
  const columnOf = (i: number) => (rowOf(i) % 2 === 0 ? i % columns : columns - 1 - (i % columns))
  const columnWidths: number[] = []
  p.stages.forEach((stage, i) => {
    columnWidths[columnOf(i)] = Math.max(columnWidths[columnOf(i)] ?? 0, sizeOf(stage).width)
  })
  const columnX = (column: number) => columnWidths.slice(0, column).reduce((sum, w) => sum + (w ?? 0) + gap, 0)
  const graphWidth = columnX(columnWidths.length - 1) + (columnWidths.at(-1) ?? 0)

  p.stages.forEach((stage, i) => {
    const isBuild = stage.id === 'build'
    const { width, height } = sizeOf(stage)
    const current = stage.id === p.current
    const row = rowOf(i)
    nodes.push({
      id: stageId(stage.id),
      type: 'focusStage',
      position: { x: columnX(columnOf(i)), y: rowTop(row) },
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
      const [sourceHandle, targetHandle] = forwardHandles(rowOf(i - 1), row)
      edges.push({
        id: `forward-${prev.id}-${stage.id}`,
        source: stageId(prev.id),
        target: stageId(stage.id),
        sourceHandle,
        targetHandle,
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

  })

  const height = rowTop(rowHeights.length - 1) + (rowHeights.at(-1) ?? 0) + bottomSpace
  return { nodes, edges, width: graphWidth, height }
}

// forwardHandles are the handles a forward edge uses: right to left along
// an even row, left to right along an odd one, bottom to top across rows.
function forwardHandles(fromRow: number, toRow: number): [string, string] {
  if (fromRow !== toRow) return ['wrap-out', 'wrap-in']
  return toRow % 2 === 0 ? ['out', 'in'] : ['out-left', 'in-right']
}

// focusGraphColumns is the most stages per row whose graph, fitted to a pane
// paneWidth wide, stays at GRAPH_MIN_ZOOM or closer: one row on a wide
// screen, wrapped rows on a narrow one.
export function focusGraphColumns(p: FocusPipeline, labels: FocusGraphLabels, paneWidth: number): number {
  const most = Math.max(1, p.stages.length)
  for (let columns = most; columns > 1; columns--) {
    const { width } = buildFocusGraph(p, labels, { columns })
    if (paneWidth / (width * (1 + 2 * GRAPH_PADDING)) >= GRAPH_MIN_ZOOM) return columns
  }
  return 1
}
