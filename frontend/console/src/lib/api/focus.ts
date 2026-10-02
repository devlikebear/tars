import { APIRequestError, requestJSON } from './client.ts'
import type {
  FocusActionResult,
  FocusCardState,
  FocusGateAction,
  FocusListItem,
  FocusPipeline,
  FocusPlan,
  FocusPRDraft,
  FocusQAResult,
  FocusStageId,
  ReleaseTrain,
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

// kind 'release' marks a pipeline started from the release train; kickoff
// is a first turn that says more than the goal (the merged list).
export type FocusCreateRequest = {
  goal: string
  cwd: string
  isolate?: boolean
  title?: string
  kind?: 'release'
  kickoff?: string
  // A release's list and the cut-off it started from (the group's since).
  release_items?: string[]
  release_since?: string
}

// startFocusRelease creates a release pipeline; when the repository already
// runs one (409), it answers that release's session instead.
export async function startFocusRelease(request: FocusCreateRequest): Promise<{ session_id: string; existing: boolean }> {
  try {
    const created = await createFocusPipeline(request)
    return { session_id: created.session_id, existing: false }
  } catch (err) {
    const running = err instanceof APIRequestError && err.status === 409 ? (err.payload as { session_id?: string } | undefined)?.session_id : undefined
    if (running) return { session_id: running, existing: true }
    throw err
  }
}

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
  options: { note?: string; edits?: FocusPlan; pr?: FocusPRDraft } = {},
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

export function getReleaseTrain(): Promise<ReleaseTrain> {
  return requestJSON<ReleaseTrain>('/v1/focus/release-train')
}

// askFocusQuestion asks the pipeline's Q&A session about one card (ADR §8).
// The answer streams on GET /v1/chat/stream?session_id=<qa_session_id>.
export function askFocusQuestion(sessionId: string, cardId: string, question: string): Promise<FocusQAResult> {
  return requestJSON<FocusQAResult>(`${pipelinePath(sessionId)}/qa`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ card_id: cardId, question }),
  })
}
