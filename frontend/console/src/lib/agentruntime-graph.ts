import type { AgentRuntimeRunEvent } from './types'

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
