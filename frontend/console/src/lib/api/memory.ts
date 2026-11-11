import { requestJSON } from './client.ts'
import type {
  LocalEmbedAction,
  LocalEmbedState,
  MemoryAsset,
  MemoryCandidateAction,
  MemoryCandidateListResponse,
  MemoryCandidateReviewResponse,
  MemoryCandidateStatus,
  MemoryFile,
  MemoryPrefetchResult,
  MemorySearchResult,
  SyspromptFile,
  SyspromptPreview,
} from '../types'

export async function listMemoryAssets(): Promise<{ count: number; items: MemoryAsset[] }> {
  return requestJSON<{ count: number; items: MemoryAsset[] }>('/v1/memory/assets')
}

export async function getMemoryFile(path: string): Promise<MemoryFile> {
  return requestJSON<MemoryFile>(`/v1/memory/file?path=${encodeURIComponent(path)}`)
}

export async function saveMemoryFile(path: string, content: string): Promise<MemoryFile> {
  return requestJSON<MemoryFile>('/v1/memory/file', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

export async function listMemoryInbox(status: MemoryCandidateStatus | 'all' = 'pending'): Promise<MemoryCandidateListResponse> {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  const qs = params.toString()
  return requestJSON<MemoryCandidateListResponse>(`/v1/memory/inbox${qs ? `?${qs}` : ''}`)
}

export async function reviewMemoryCandidate(
  id: string,
  action: MemoryCandidateAction,
  mergeTarget?: string,
): Promise<MemoryCandidateReviewResponse> {
  return requestJSON<MemoryCandidateReviewResponse>('/v1/memory/inbox/review', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, action, merge_target: mergeTarget || '' }),
  })
}

// The answer to a confirm_memory question (tars#1001): the candidate's inbox
// status, or "missing" once it is gone from the inbox.
export type InitiativeMemoryAnswer = { entry_id: string; status: MemoryCandidateStatus | 'missing' }

export async function getInitiativeMemoryAnswer(entryId: string): Promise<InitiativeMemoryAnswer> {
  return requestJSON<InitiativeMemoryAnswer>(`/v1/initiative/memory/answer?entry_id=${encodeURIComponent(entryId)}`)
}

export async function answerInitiativeMemory(entryId: string, action: 'approve' | 'reject'): Promise<InitiativeMemoryAnswer> {
  return requestJSON<InitiativeMemoryAnswer>('/v1/initiative/memory/answer', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry_id: entryId, action }),
  })
}

// A check-in's answer buttons in CASE's bubble (case-companion phase 3).
// The button is the answer — the server records it without reading any
// words. `until` is when a snooze ends.
export type InitiativeCheckInAction = 'ok' | 'snooze' | 'quiet_today'
export type InitiativeCheckInAnswer = { entry_id: string; action: InitiativeCheckInAction; until?: string }

export async function answerInitiativeCheckIn(entryId: string, action: InitiativeCheckInAction): Promise<InitiativeCheckInAnswer> {
  return requestJSON<InitiativeCheckInAnswer>('/v1/initiative/checkin/answer', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry_id: entryId, action }),
  })
}

export async function runMemorySearch(payload: {
  query: string
  limit?: number
  include_memory?: boolean
  include_daily?: boolean
  include_sessions?: boolean
}): Promise<MemorySearchResult> {
  return requestJSON<MemorySearchResult>('/v1/memory/search', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function runMemoryPrefetch(payload: {
  query: string
  session_id?: string
}): Promise<MemoryPrefetchResult> {
  const params = new URLSearchParams({ query: payload.query })
  if (payload.session_id?.trim()) params.set('session_id', payload.session_id.trim())
  return requestJSON<MemoryPrefetchResult>(`/v1/memory/prefetch?${params.toString()}`, {
    method: 'POST',
  })
}

// --- System prompt workspace ---

export async function listSyspromptFiles(scope?: 'workspace' | 'agent'): Promise<{ count: number; items: SyspromptFile[] }> {
  const qs = scope ? `?scope=${encodeURIComponent(scope)}` : ''
  return requestJSON<{ count: number; items: SyspromptFile[] }>(`/v1/workspace/sysprompt/files${qs}`)
}

export async function getSyspromptFile(scope: 'workspace' | 'agent', path: string): Promise<SyspromptFile> {
  return requestJSON<SyspromptFile>(`/v1/workspace/sysprompt/file?scope=${encodeURIComponent(scope)}&path=${encodeURIComponent(path)}`)
}

export async function saveSyspromptFile(scope: 'workspace' | 'agent', path: string, content: string): Promise<SyspromptFile> {
  return requestJSON<SyspromptFile>('/v1/workspace/sysprompt/file', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ scope, path, content }),
  })
}

export async function getSyspromptPreview(target: 'main_agent' | 'sub_agent' = 'main_agent'): Promise<SyspromptPreview> {
  return requestJSON<SyspromptPreview>(`/v1/admin/sysprompt/preview?target=${encodeURIComponent(target)}`)
}

export async function getLocalEmbed(signal?: AbortSignal): Promise<LocalEmbedState> {
  return requestJSON<LocalEmbedState>('/v1/admin/local-embed', { signal })
}

export async function runLocalEmbedAction(action: LocalEmbedAction): Promise<LocalEmbedState> {
  return requestJSON<LocalEmbedState>(`/v1/admin/local-embed/${action}`, { method: 'POST' })
}
