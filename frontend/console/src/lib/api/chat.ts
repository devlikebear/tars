import { requestJSON } from './client.ts'
import type {
  APIErrorPayload,
  ChatEvent,
  ChatRequest,
  ChatTierRecommendationRequest,
  CommandDef,
  NotificationMessage,
  SessionStyleResponse,
} from '../types'

// --- Events (singleton SSE) ---
//
// A single EventSource is shared across all components to avoid exhausting the
// browser's per-origin HTTP/1.1 connection limit (typically 6).  Components
// call streamEvents() to subscribe and receive a cleanup function that
// unsubscribes without closing the underlying connection.

type EventListener = {
  onEvent: (event: NotificationMessage) => void
  onError?: (message: string) => void
  onOpen?: () => void
  onReopen?: () => void
}

let sharedStream: EventSource | null = null
let listeners = new Map<number, EventListener>()
let nextListenerId = 0
let hasOpenedOnce = false
let reconnectAttempt = 0
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

function clearReconnectTimer() {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function scheduleReconnect() {
  if (reconnectTimer !== null) return
  if (listeners.size === 0) return
  // Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s.
  const delay = Math.min(30_000, 1_000 * 2 ** Math.min(reconnectAttempt, 5))
  reconnectAttempt += 1
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    if (listeners.size === 0) return
    ensureStream()
  }, delay)
}

function ensureStream() {
  if (sharedStream && sharedStream.readyState !== EventSource.CLOSED) return
  if (sharedStream) {
    sharedStream.close()
    sharedStream = null
  }
  const wasOpenedBefore = hasOpenedOnce
  sharedStream = new EventSource('/v1/events/stream', { withCredentials: true })
  sharedStream.onopen = () => {
    clearReconnectTimer()
    reconnectAttempt = 0
    const isReopen = wasOpenedBefore
    hasOpenedOnce = true
    for (const l of listeners.values()) {
      l.onOpen?.()
      if (isReopen) l.onReopen?.()
    }
  }
  sharedStream.onmessage = (message) => {
    if (!message.data) return
    try {
      const payload = JSON.parse(message.data) as NotificationMessage
      if (payload.type === 'keepalive') return
      for (const l of listeners.values()) l.onEvent(payload)
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Failed to parse event stream payload'
      for (const l of listeners.values()) l.onError?.(msg)
    }
  }
  sharedStream.onerror = () => {
    for (const l of listeners.values()) l.onError?.('Event stream disconnected')
    // EventSource auto-reconnects on transient drops (readyState=CONNECTING).
    // Only schedule a manual reconnect once it gives up and reaches CLOSED.
    if (sharedStream?.readyState === EventSource.CLOSED) {
      scheduleReconnect()
    }
  }
}

function maybeCloseStream() {
  if (listeners.size === 0 && sharedStream) {
    sharedStream.close()
    sharedStream = null
    clearReconnectTimer()
    reconnectAttempt = 0
    hasOpenedOnce = false
  }
}

export function streamEvents(
  onEvent: (event: NotificationMessage) => void,
  onError?: (message: string) => void,
  onOpen?: () => void,
  onReopen?: () => void,
): () => void {
  const id = nextListenerId++
  listeners.set(id, { onEvent, onError, onOpen, onReopen })
  ensureStream()
  return () => {
    listeners.delete(id)
    maybeCloseStream()
  }
}

export async function streamChat(
  request: ChatRequest,
  onEvent: (event: ChatEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const response = await fetch('/v1/chat', {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
    signal,
  })

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim()
    try {
      const payload = (await response.json()) as APIErrorPayload
      if (payload?.error?.trim()) {
        message = payload.error.trim()
      }
    } catch {
      // ignore non-JSON error bodies
    }
    throw new Error(message)
  }

  if (!response.body) {
    throw new Error('chat stream body missing')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() ?? ''

    for (const line of lines) {
      if (!line.startsWith('data:')) {
        continue
      }
      const payload = line.slice(5).trim()
      if (!payload) {
        continue
      }
      onEvent(JSON.parse(payload) as ChatEvent)
    }

    if (done) {
      break
    }
  }
}

export async function cancelChat(sessionId: string): Promise<boolean> {
  try {
    const result = await requestJSON<{ cancelled: boolean }>(
      `/v1/chat/cancel?session_id=${encodeURIComponent(sessionId)}`,
      { method: 'POST' },
    )
    return result.cancelled
  } catch {
    return false
  }
}

export type ChatToolInfo = {
  name: string
  description: string
  high_risk: boolean
  group?: string
}

export type ChatToolsResponse = {
  tools: ChatToolInfo[]
  skills?: string[]
  commands?: CommandDef[]
  mcp_servers?: string[]
}

export async function listChatTools(sessionId?: string): Promise<ChatToolsResponse> {
  const params = new URLSearchParams()
  if (sessionId?.trim()) params.set('session_id', sessionId.trim())
  const qs = params.toString()
  return requestJSON<ChatToolsResponse>(`/v1/chat/tools${qs ? `?${qs}` : ''}`)
}

// --- Chat Context ---

export type ChatContextInfo = {
  session_id: string
  system_prompt: string
  system_prompt_tokens: number
  history_tokens: number
  history_messages: number
  tool_count: number
  tool_names: string[]
  skill_count?: number
  skill_names?: string[]
  command_count?: number
  command_names?: string[]
  memory_count: number
  memory_tokens: number
  compaction_trigger_tokens?: number
  compaction_keep_recent_tokens?: number
  compaction_keep_recent_fraction?: number
  compaction_last_mode?: string
  used_tool_names?: string[]
  selected_skill_name?: string
  selected_skill_reason?: string
  selected_command_name?: string
  selected_command_reason?: string
  mentioned_path_count?: number
  mentioned_paths?: string[]
  mentioned_subagent_count?: number
  mentioned_subagents?: string[]
  llm_tier?: string
  tier_recommendation?: ChatTierRecommendationRequest
  style_effective?: SessionStyleResponse['effective']
  prompt_override: string
}

export async function getChatContext(sessionId: string): Promise<ChatContextInfo> {
  return requestJSON<ChatContextInfo>(`/v1/chat/context?session_id=${encodeURIComponent(sessionId)}`)
}

export type PriorContextPreviewItem = {
  source: string
  source_tag: string
  snippet: string
  tokens: number
}

export type PriorContextPreviewMode = 'default' | 'recent'

export type PriorContextPreview = {
  session_id: string
  query: string
  mode: PriorContextPreviewMode
  section: string
  items: PriorContextPreviewItem[]
  below_threshold_items: PriorContextPreviewItem[]
  recent_fallback_items: PriorContextPreviewItem[]
  relevant_tokens: number
  relevant_memory_count: number
  relevant_budget_tokens: number
  budget_percent: number
  generated_at: string
}

export async function getPriorContextPreview(sessionId: string, query: string): Promise<PriorContextPreview> {
  return requestJSON<PriorContextPreview>('/v1/chat/prior-context/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, query }),
  })
}

export type ChatFileMentionCandidate = {
  kind: 'file' | 'directory'
  name: string
  path: string
  root: string
  root_label: string
  token: string
  size?: number
  updated_at?: string
}

export async function listChatFileMentions(
  sessionId: string | undefined,
  query: string,
  limit = 30,
): Promise<{ candidates: ChatFileMentionCandidate[] }> {
  const params = new URLSearchParams({ q: query, limit: String(limit) })
  if (sessionId?.trim()) params.set('session_id', sessionId.trim())
  return requestJSON<{ candidates: ChatFileMentionCandidate[] }>(
    `/v1/chat/mentions/files?${params}`,
  )
}

export async function getSessionPrompt(sessionId: string): Promise<{ prompt_override: string }> {
  return requestJSON<{ prompt_override: string }>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/prompt`)
}

export async function updateSessionPrompt(sessionId: string, promptOverride: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/prompt`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt_override: promptOverride }),
  })
}
