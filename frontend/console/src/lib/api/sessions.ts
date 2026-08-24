import { requestJSON } from './client.ts'
import type {
  ForkPromotionListResponse,
  ForkPromotionResult,
  Session,
  SessionAutomationConsent,
  SessionCleanupMode,
  SessionCleanupSuggestionResponse,
  SessionCritic,
  SessionCwd,
  SessionEffectiveConfig,
  SessionGoal,
  SessionLocalSkillListResponse,
  SessionLocalSkillPromoteRequest,
  SessionLocalSkillPromoteResponse,
  SessionMessage,
  SessionStyleControl,
  SessionStyleResponse,
  SessionWorkDirs,
} from '../types'

export type SessionArchiveMode = 'active' | 'include' | 'only'

export async function listSessions(includeHidden = false, archivedMode: SessionArchiveMode = 'active'): Promise<Session[]> {
  const params = new URLSearchParams()
  if (includeHidden) params.set('hidden', '1')
  if (archivedMode !== 'active') params.set('archived', archivedMode)
  const query = params.toString()
  return requestJSON<Session[]>(`/v1/admin/sessions${query ? `?${query}` : ''}`)
}

export async function createSession(title?: string): Promise<Session> {
  return requestJSON<Session>('/v1/admin/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: title || 'New Chat' }),
  })
}

export async function getSession(sessionId: string): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`)
}

export async function getSessionHistory(sessionId: string): Promise<SessionMessage[]> {
  return requestJSON<SessionMessage[]>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/history`)
}

export async function forkSessionFromMessage(sessionId: string, messageId: string, forkReason?: string): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/fork`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message_id: messageId, fork_reason: forkReason || 'Forked from chat transcript' }),
  })
}

export async function getForkPromotions(sessionId: string): Promise<ForkPromotionListResponse> {
  return requestJSON<ForkPromotionListResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/promotions`)
}

export async function promoteForkInsights(sessionId: string, candidateIds: string[]): Promise<ForkPromotionResult> {
  return requestJSON<ForkPromotionResult>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/promotions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ candidate_ids: candidateIds }),
  })
}

export async function renameSession(sessionId: string, title: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export async function setSessionArchived(sessionId: string, archived: boolean): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ archived }),
  })
}

export async function setSessionPinned(sessionId: string, pinned: boolean): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pinned }),
  })
}

export async function recommendSessionCleanup(request: { mode: SessionCleanupMode; limit?: number }): Promise<SessionCleanupSuggestionResponse> {
  return requestJSON<SessionCleanupSuggestionResponse>('/v1/admin/sessions/cleanup-suggestions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  })
}

export async function deleteSession(sessionId: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'DELETE',
  })
}

export interface CompactResult {
  session_id: string
  compacted: boolean
  original_count: number
  final_count: number
  compacted_count: number
  tokens_before: number
  tokens_after: number
  reason: string
}

export async function compactSession(sessionId: string): Promise<CompactResult> {
  return requestJSON<CompactResult>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/compact`,
    { method: 'POST' },
  )
}

export async function getSessionWorkDirs(sessionId: string): Promise<SessionWorkDirs> {
  return requestJSON<SessionWorkDirs>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/workdirs`)
}

export async function updateSessionWorkDirs(sessionId: string, data: { work_dirs: string[]; current_dir: string }): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/workdirs`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function getSessionCwd(sessionId: string): Promise<SessionCwd> {
  return requestJSON<SessionCwd>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/cwd`)
}

export async function setSessionCwd(sessionId: string, current: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/cwd`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current }),
  })
}

export async function getSessionGoal(sessionId: string): Promise<{ goal: SessionGoal | null }> {
  return requestJSON<{ goal: SessionGoal | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/goal`,
  )
}

export async function setSessionGoal(
  sessionId: string,
  description: string,
  maxAutoContinues?: number,
): Promise<{ goal: SessionGoal | null }> {
  const body: Record<string, unknown> = { description }
  if (typeof maxAutoContinues === 'number' && maxAutoContinues > 0) {
    body.max_auto_continues = maxAutoContinues
  }
  return requestJSON<{ goal: SessionGoal | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/goal`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  )
}

export async function clearSessionGoal(sessionId: string): Promise<{ goal: SessionGoal | null }> {
  return requestJSON<{ goal: SessionGoal | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/goal`,
    { method: 'DELETE' },
  )
}

export async function getSessionCritic(
  sessionId: string,
): Promise<{ critic: SessionCritic | null }> {
  return requestJSON<{ critic: SessionCritic | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/critic`,
  )
}

export async function setSessionCritic(
  sessionId: string,
  enabled: boolean,
  maxIterations?: number,
): Promise<{ critic: SessionCritic | null }> {
  const body: Record<string, unknown> = { enabled }
  if (typeof maxIterations === 'number' && maxIterations > 0) {
    body.max_iterations = maxIterations
  }
  return requestJSON<{ critic: SessionCritic | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/critic`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  )
}

export async function clearSessionCritic(
  sessionId: string,
): Promise<{ critic: SessionCritic | null }> {
  return requestJSON<{ critic: SessionCritic | null }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/critic`,
    { method: 'DELETE' },
  )
}

export async function getSessionEffectiveConfig(sessionId: string): Promise<SessionEffectiveConfig> {
  return requestJSON<SessionEffectiveConfig>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/effective-config`,
  )
}

// --- Session Config ---

export type SessionToolConfig = {
  tools_enabled?: string[]
  tools_custom?: boolean
  tools_disabled?: string[]
  tools_allow_groups?: string[]
  tools_deny_groups?: string[]
  skills_enabled?: string[]
  skills_custom?: boolean
  commands_enabled?: string[]
  commands_custom?: boolean
  mcp_enabled?: string[]
  mcp_custom?: boolean
}

export async function getSessionConfig(sessionId: string): Promise<SessionToolConfig> {
  return requestJSON<SessionToolConfig>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/config`)
}

export async function updateSessionConfig(sessionId: string, config: SessionToolConfig): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/config`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
}

export async function updateSessionLocalConfig(
  sessionId: string,
  config: SessionToolConfig,
): Promise<SessionEffectiveConfig> {
  return requestJSON<SessionEffectiveConfig>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/local-config`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
}

export async function getSessionAutomationConsent(sessionId: string): Promise<SessionAutomationConsent> {
  return requestJSON<SessionAutomationConsent>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/automation-consent`)
}

export async function updateSessionAutomationConsent(
  sessionId: string,
  consent: SessionAutomationConsent,
): Promise<SessionAutomationConsent> {
  return requestJSON<SessionAutomationConsent>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/automation-consent`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(consent),
  })
}

export async function getSessionStyle(sessionId: string): Promise<SessionStyleResponse> {
  return requestJSON<SessionStyleResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/style`)
}

export async function updateSessionStyle(
  sessionId: string,
  style: SessionStyleControl,
): Promise<SessionStyleResponse> {
  return requestJSON<SessionStyleResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/style`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(style),
  })
}

export async function listSessionLocalSkills(sessionId: string): Promise<SessionLocalSkillListResponse> {
  return requestJSON<SessionLocalSkillListResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/local-skills`)
}

export async function promoteSessionLocalSkills(
  sessionId: string,
  payload: SessionLocalSkillPromoteRequest,
): Promise<SessionLocalSkillPromoteResponse> {
  return requestJSON<SessionLocalSkillPromoteResponse>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/local-skills/promote`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
  )
}

// --- Terminal (session cwd control) ---

export type OpenTerminalResult = {
  ok: boolean
  cwd: string
  app: string
  message?: string
}

export async function openTerminalHere(sessionId: string, cwd?: string): Promise<OpenTerminalResult> {
  return requestJSON<OpenTerminalResult>('/v1/terminal/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, cwd: cwd || '' }),
  })
}

export function terminalWebSocketURL(sessionId: string, cwd?: string, cols = 80, rows = 24): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const params = new URLSearchParams({
    session_id: sessionId,
    cols: String(cols),
    rows: String(rows),
  })
  if (cwd) params.set('cwd', cwd)
  return `${protocol}//${window.location.host}/v1/terminal/ws?${params}`
}
