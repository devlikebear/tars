import { requestJSON } from './client.ts'
import { normalizeGlobalPlans, normalizePlanArchive, normalizeSessionTasks } from './normalize.ts'
import type {
  GlobalPlansResponse,
  PlanArchiveResponse,
  SessionTasks,
  TaskVerificationResponse,
  WorkLedgerEvent,
  WorkLedgerProjection,
  WorkLedgerWork,
  WorkLedgerWorksResponse,
} from '../types'

export async function getSessionTasks(sessionId: string): Promise<SessionTasks> {
  const data = await requestJSON<Partial<SessionTasks>>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks`)
  return normalizeSessionTasks(data)
}

export async function getSessionWorkLedger(sessionId: string): Promise<WorkLedgerWork | null> {
  for (const source of ['session', 'legacy-session']) {
    const params = new URLSearchParams({
      source,
      source_id: sessionId,
      limit: '1',
    })
    const data = await requestJSON<WorkLedgerWorksResponse>(`/v1/work/works?${params.toString()}`)
    if (data.works[0]) return data.works[0]
  }
  return null
}

export async function getWorkLedgerTimeline(workId: string): Promise<WorkLedgerProjection> {
  return requestJSON<WorkLedgerProjection>(`/v1/work/works/${encodeURIComponent(workId)}/timeline`)
}

export async function cancelWorkLedger(workId: string, reason: string): Promise<WorkLedgerProjection> {
  return requestJSON<WorkLedgerProjection>(`/v1/admin/work/works/${encodeURIComponent(workId)}/cancel`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason }),
  })
}

export async function resumeWorkLedgerStep(workId: string, stepId: string, reason: string): Promise<WorkLedgerProjection> {
  return requestJSON<WorkLedgerProjection>(
    `/v1/admin/work/works/${encodeURIComponent(workId)}/steps/${encodeURIComponent(stepId)}/resume`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason }),
    },
  )
}

export function watchWorkLedger(
  workId: string,
  afterSequence: number,
  onEvent: (event: WorkLedgerEvent) => void,
  onError?: () => void,
): () => void {
  const params = new URLSearchParams({ after_sequence: String(Math.max(0, afterSequence)) })
  const stream = new EventSource(
    `/v1/work/works/${encodeURIComponent(workId)}/watch?${params.toString()}`,
    { withCredentials: true },
  )
  stream.addEventListener('work_event', (message) => {
    if (!(message instanceof MessageEvent) || !message.data) return
    try {
      onEvent(JSON.parse(message.data) as WorkLedgerEvent)
    } catch {
      onError?.()
    }
  })
  stream.onerror = () => onError?.()
  return () => stream.close()
}

export async function getGlobalPlans(active = true): Promise<GlobalPlansResponse> {
  const endpoint = active ? '/v1/admin/tasks?active=true' : '/v1/admin/tasks?active=false'
  const data = await requestJSON<Partial<GlobalPlansResponse>>(endpoint)
  return normalizeGlobalPlans(data)
}

export async function getPlanArchive(limit = 50): Promise<PlanArchiveResponse> {
  const data = await requestJSON<Partial<PlanArchiveResponse>>(`/v1/admin/plans/archive?limit=${limit}`)
  return normalizePlanArchive(data)
}

export async function getSessionPlanArchive(sessionId: string, limit = 20): Promise<PlanArchiveResponse> {
  const data = await requestJSON<Partial<PlanArchiveResponse>>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/plans/archive?limit=${limit}`,
  )
  return normalizePlanArchive(data)
}

// executeTasksAction drives the plan state machine directly from the
// console — used by the TasksPanel CTA buttons (Approve / Discard / Save
// edits). The body shape mirrors the chat-side `tasks` tool action.
export async function executeTasksAction(
  sessionId: string,
  payload: Record<string, unknown>,
): Promise<unknown> {
  return requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function runTaskVerification(
  sessionId: string,
  payload: { task_id?: string; timeout_ms?: number } = {},
): Promise<TaskVerificationResponse> {
  return requestJSON<TaskVerificationResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}
