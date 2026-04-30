import type { AgentRuntimeRun, AgentRuntimeRunEvent, ConsensusVariantRecord } from './types'

export type AgentRuntimeReplayBounds = {
  startMs: number
  endMs: number
  hasTimeline: boolean
}

export type AgentRuntimeReplayState = {
  appliedCount: number
  totalCount: number
  status: string
  lastEventType: string
  lastMessage: string
  filePaths: string[]
  currentTimeMs: number
}

type TimestampedEvent = {
  event: AgentRuntimeRunEvent
  timestampMs: number
}

export type AgentRuntimeStatusKind = 'pending' | 'running' | 'done' | 'error'
export type AgentRuntimeTierShape = 'heavy' | 'standard' | 'light'

export type AgentRuntimeTreeRow = {
  run: AgentRuntimeRun
  runId: string
  parentRunId: string
  agent: string
  status: string
  statusKind: AgentRuntimeStatusKind
  tier: string
  tierShape: AgentRuntimeTierShape
  depth: number
  x: number
  y: number
}

export type AgentRuntimeGanttVariant = {
  key: string
  label: string
  statusKind: AgentRuntimeStatusKind
  leftPercent: number
  widthPercent: number
  tokens: number
}

export type AgentRuntimeGanttRow = {
  run: AgentRuntimeRun
  runId: string
  label: string
  statusKind: AgentRuntimeStatusKind
  tier: string
  startMs: number
  endMs: number
  durationMs: number
  leftPercent: number
  widthPercent: number
  variants: AgentRuntimeGanttVariant[]
}

export type AgentRuntimeGanttModel = {
  startMs: number
  endMs: number
  durationMs: number
  hasTimeline: boolean
  rows: AgentRuntimeGanttRow[]
}

export function deriveAgentRuntimeReplayBounds(events: AgentRuntimeRunEvent[]): AgentRuntimeReplayBounds {
  const timeline = timestampedEvents(events)
  if (timeline.length === 0) {
    return { startMs: 0, endMs: 0, hasTimeline: false }
  }
  return {
    startMs: timeline[0].timestampMs,
    endMs: timeline[timeline.length - 1].timestampMs,
    hasTimeline: true,
  }
}

export function deriveAgentRuntimeReplayState(events: AgentRuntimeRunEvent[], cursorMs: number): AgentRuntimeReplayState {
  const timeline = timestampedEvents(events)
  const safeCursor = Number.isFinite(cursorMs) ? cursorMs : timeline[timeline.length - 1]?.timestampMs ?? 0
  const applied = timeline.filter((item) => item.timestampMs <= safeCursor)
  const filePaths = new Set<string>()
  let status = 'pending'
  let lastEventType = 'none'
  let lastMessage = ''

  for (const item of applied) {
    const event = item.event
    status = statusFromEvent(event, status)
    lastEventType = event.type
    lastMessage = messageFromEvent(event)
    const path = event.path?.trim()
    if (path) filePaths.add(path)
  }

  return {
    appliedCount: applied.length,
    totalCount: timeline.length,
    status,
    lastEventType,
    lastMessage,
    filePaths: [...filePaths],
    currentTimeMs: safeCursor,
  }
}

export function buildAgentRuntimeTreeRows(runs: AgentRuntimeRun[]): AgentRuntimeTreeRow[] {
  const sorted = [...runs].sort(compareRunsByStart)
  const byID = new Map(sorted.map((run) => [run.run_id, run]))
  const children = new Map<string, AgentRuntimeRun[]>()
  for (const run of sorted) {
    const parent = run.parent_run_id?.trim()
    if (!parent || !byID.has(parent)) continue
    const group = children.get(parent) ?? []
    group.push(run)
    children.set(parent, group)
  }
  for (const group of children.values()) {
    group.sort(compareRunsByStart)
  }

  const roots = sorted.filter((run) => !run.parent_run_id?.trim() || !byID.has(run.parent_run_id))
  const rows: AgentRuntimeTreeRow[] = []
  const visited = new Set<string>()

  for (const root of roots) {
    appendTreeRun(root, 0, children, visited, rows)
  }
  for (const run of sorted) {
    if (!visited.has(run.run_id)) appendTreeRun(run, run.depth ?? 0, children, visited, rows)
  }

  return rows
}

export function buildAgentRuntimeGanttRows(runs: AgentRuntimeRun[]): AgentRuntimeGanttModel {
  const timedRuns = runs
    .map((run) => {
      const startMs = runStartMs(run)
      if (startMs == null) return null
      const endMs = Math.max(startMs, runEndMs(run) ?? startMs)
      return { run, startMs, endMs }
    })
    .filter((item): item is { run: AgentRuntimeRun; startMs: number; endMs: number } => item != null)
    .sort((a, b) => a.startMs - b.startMs || a.run.run_id.localeCompare(b.run.run_id))

  if (timedRuns.length === 0) {
    return { startMs: 0, endMs: 0, durationMs: 0, hasTimeline: false, rows: [] }
  }

  const startMs = Math.min(...timedRuns.map((item) => item.startMs))
  const rawEndMs = Math.max(...timedRuns.map((item) => item.endMs))
  const endMs = rawEndMs > startMs ? rawEndMs : startMs + 60_000
  const durationMs = endMs - startMs
  const rows = timedRuns.map((item) => {
    const duration = Math.max(1, item.endMs - item.startMs)
    return {
      run: item.run,
      runId: item.run.run_id,
      label: item.run.agent || shortID(item.run.run_id),
      statusKind: statusKindFromStatus(item.run.status),
      tier: item.run.tier || 'default',
      startMs: item.startMs,
      endMs: item.endMs,
      durationMs: duration,
      leftPercent: percentBetween(item.startMs, startMs, durationMs),
      widthPercent: Math.max(1, percentDuration(duration, durationMs)),
      variants: buildVariantBars(item.run, startMs, durationMs),
    }
  })

  return { startMs, endMs, durationMs, hasTimeline: true, rows }
}

function timestampedEvents(events: AgentRuntimeRunEvent[]): TimestampedEvent[] {
  return events
    .map((event) => ({ event, timestampMs: parseTimestamp(event.timestamp) }))
    .filter((item): item is TimestampedEvent => item.timestampMs != null)
    .sort((a, b) => a.timestampMs - b.timestampMs)
}

function parseTimestamp(value?: string): number | null {
  if (!value?.trim()) return null
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? null : parsed
}

function appendTreeRun(
  run: AgentRuntimeRun,
  fallbackDepth: number,
  children: Map<string, AgentRuntimeRun[]>,
  visited: Set<string>,
  rows: AgentRuntimeTreeRow[],
) {
  if (visited.has(run.run_id)) return
  visited.add(run.run_id)
  const depth = Math.max(0, run.depth ?? fallbackDepth)
  rows.push({
    run,
    runId: run.run_id,
    parentRunId: run.parent_run_id ?? '',
    agent: run.agent || 'default',
    status: run.status || 'pending',
    statusKind: statusKindFromStatus(run.status),
    tier: run.tier || 'default',
    tierShape: tierShape(run.tier),
    depth,
    x: 28 + depth * 72,
    y: 40 + rows.length * 78,
  })
  for (const child of children.get(run.run_id) ?? []) {
    appendTreeRun(child, depth + 1, children, visited, rows)
  }
}

function buildVariantBars(run: AgentRuntimeRun, timelineStartMs: number, timelineDurationMs: number): AgentRuntimeGanttVariant[] {
  const runStart = runStartMs(run) ?? timelineStartMs
  return [...(run.consensus_variants ?? [])]
    .sort((a, b) => a.variant_idx - b.variant_idx)
    .map((variant) => variantBar(run.run_id, variant, runStart, timelineStartMs, timelineDurationMs))
}

function variantBar(
  runID: string,
  variant: ConsensusVariantRecord,
  fallbackStartMs: number,
  timelineStartMs: number,
  timelineDurationMs: number,
): AgentRuntimeGanttVariant {
  const startMs = parseTimestamp(variant.started_at) ?? fallbackStartMs
  const endMs = Math.max(startMs, parseTimestamp(variant.finished_at) ?? startMs)
  const durationMs = Math.max(1, endMs - startMs)
  return {
    key: `${runID}-${variant.variant_idx}`,
    label: variant.alias || `v${variant.variant_idx + 1}`,
    statusKind: statusKindFromStatus(variant.status),
    leftPercent: percentBetween(startMs, timelineStartMs, timelineDurationMs),
    widthPercent: Math.max(1, percentDuration(durationMs, timelineDurationMs)),
    tokens: (variant.tokens_in ?? 0) + (variant.tokens_out ?? 0),
  }
}

function runStartMs(run: AgentRuntimeRun): number | null {
  for (const value of [run.started_at, run.created_at, run.updated_at, run.completed_at]) {
    const parsed = parseTimestamp(value)
    if (parsed != null) return parsed
  }
  return null
}

function runEndMs(run: AgentRuntimeRun): number | null {
  for (const value of [run.completed_at, run.updated_at, run.started_at, run.created_at]) {
    const parsed = parseTimestamp(value)
    if (parsed != null) return parsed
  }
  return null
}

function compareRunsByStart(a: AgentRuntimeRun, b: AgentRuntimeRun): number {
  return (runStartMs(a) ?? 0) - (runStartMs(b) ?? 0) || a.run_id.localeCompare(b.run_id)
}

function percentBetween(valueMs: number, startMs: number, durationMs: number): number {
  if (durationMs <= 0) return 0
  return clampPercent(((valueMs - startMs) / durationMs) * 100)
}

function percentDuration(valueMs: number, durationMs: number): number {
  if (durationMs <= 0) return 100
  return clampPercent((valueMs / durationMs) * 100)
}

function clampPercent(value: number): number {
  return Math.max(0, Math.min(100, value))
}

function shortID(value?: string): string {
  const text = value?.trim()
  if (!text) return 'run'
  return text.length > 12 ? `${text.slice(0, 12)}...` : text
}

function tierShape(tier?: string): AgentRuntimeTierShape {
  const normalized = (tier || '').toLowerCase()
  if (normalized === 'heavy') return 'heavy'
  if (normalized === 'light') return 'light'
  return 'standard'
}

function statusKindFromStatus(status?: string): AgentRuntimeStatusKind {
  const normalized = (status || '').toLowerCase()
  if (normalized.includes('fail') || normalized.includes('error') || normalized.includes('cancel')) return 'error'
  if (normalized.includes('complete') || normalized.includes('done') || normalized.includes('success')) return 'done'
  if (normalized.includes('run') || normalized.includes('progress') || normalized.includes('start')) return 'running'
  return 'pending'
}

function statusFromEvent(event: AgentRuntimeRunEvent, fallback: string): string {
  if (event.status?.trim()) return event.status
  if (event.type === 'run_started') return 'running'
  if (event.type === 'run_finished') return 'completed'
  if (event.type === 'run_failed') return 'failed'
  if (event.type === 'run_canceled') return 'canceled'
  return fallback
}

function messageFromEvent(event: AgentRuntimeRunEvent): string {
  return event.message
    || event.status
    || event.error
    || event.response
    || event.path
    || event.resolved_alias
    || event.tool_name
    || ''
}
