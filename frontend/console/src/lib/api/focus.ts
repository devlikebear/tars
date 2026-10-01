import { APIRequestError, requestJSON } from './client.ts'
import type {
  FocusActionResult,
  FocusCardState,
  FocusGateAction,
  FocusListItem,
  FocusPipeline,
  FocusPlan,
  FocusStageId,
} from '../types'

// --- Focus mode (docs/decisions/focus-mode.md) ---
//
// A pipeline is a sidecar of one chat session. Every mutating route answers
// {pipeline, next_prompt}; a conflict (a gate that is no longer open, a card
// already decided, a stage that moved on) is a 409 carrying the server's
// current pipeline, returned here as a result with `conflict: true` so the
// caller replaces its state instead of failing.

const base = '/v1/focus/pipelines'

function pipelinePath(sessionId: string): string {
  return `${base}/${encodeURIComponent(sessionId)}`
}

export function listFocusPipelines(): Promise<FocusListItem[]> {
  return requestJSON<FocusListItem[]>(base)
}

export function getFocusPipeline(sessionId: string): Promise<FocusPipeline> {
  return requestJSON<FocusPipeline>(pipelinePath(sessionId))
}

export type FocusCreateRequest = { goal: string; cwd: string; isolate?: boolean; title?: string }

export function createFocusPipeline(request: FocusCreateRequest): Promise<{ session_id: string; pipeline: FocusPipeline }> {
  return requestJSON(base, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  })
}

async function postAction(path: string, body: unknown): Promise<FocusActionResult> {
  try {
    return await requestJSON<FocusActionResult>(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  } catch (err) {
    const pipeline = (err instanceof APIRequestError ? (err.payload as { pipeline?: FocusPipeline } | undefined)?.pipeline : undefined)
    if (err instanceof APIRequestError && err.status === 409 && pipeline) {
      return { pipeline, next_prompt: '', conflict: true }
    }
    throw err
  }
}

export function focusGate(
  sessionId: string,
  gate: string,
  action: FocusGateAction,
  options: { note?: string; edits?: FocusPlan } = {},
): Promise<FocusActionResult> {
  return postAction(`${pipelinePath(sessionId)}/gates/${encodeURIComponent(gate)}`, { action, ...options })
}

export function focusCard(sessionId: string, cardId: string, state: FocusCardState, decision?: string): Promise<FocusActionResult> {
  return postAction(`${pipelinePath(sessionId)}/cards/${encodeURIComponent(cardId)}`, decision ? { state, decision } : { state })
}

export function focusAdvance(sessionId: string, stage: FocusStageId): Promise<FocusActionResult> {
  return postAction(`${pipelinePath(sessionId)}/advance`, { stage })
}

export function focusStop(sessionId: string): Promise<FocusActionResult> {
  return postAction(`${pipelinePath(sessionId)}/stop`, {})
}
