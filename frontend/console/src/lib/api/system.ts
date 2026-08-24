import { requestJSON } from './client.ts'
import type {
  AnalyticsResponse,
  AuthLoginRequest,
  AuthPairingLoginRequest,
  AuthWhoamiResponse,
  CodexUsageResponse,
  EventsHistoryInfo,
  HealthzResponse,
  LogsResponse,
  SetupStatusResponse,
  UsageToday,
} from '../types'

export async function getHealthz(): Promise<HealthzResponse> {
  return requestJSON<HealthzResponse>('/v1/healthz')
}

export async function getSetupStatus(): Promise<SetupStatusResponse> {
  return requestJSON<SetupStatusResponse>('/v1/setup/status')
}

export async function getServerStatus(): Promise<{ version: string }> {
  return requestJSON<{ version: string }>('/v1/status')
}

export async function getAuthWhoami(): Promise<AuthWhoamiResponse> {
  return requestJSON<AuthWhoamiResponse>('/v1/auth/whoami')
}

export async function loginAuth(payload: AuthLoginRequest): Promise<AuthWhoamiResponse> {
  return requestJSON<AuthWhoamiResponse>('/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function loginWithPairingCode(payload: AuthPairingLoginRequest): Promise<AuthWhoamiResponse> {
  return requestJSON<AuthWhoamiResponse>('/v1/auth/pairing-login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function logoutAuth(): Promise<void> {
  await requestJSON<{ ok: boolean }>('/v1/auth/logout', { method: 'POST' })
}

export async function changeBrowserPassword(role: 'admin' | 'user', payload: { current_password?: string; new_password: string }): Promise<void> {
  await requestJSON<{ ok: boolean }>(`/v1/auth/users/${encodeURIComponent(role)}/password`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function getTodayUsage(): Promise<UsageToday> {
  return requestJSON<UsageToday>('/v1/admin/usage/today')
}

export async function getCodexUsage(): Promise<CodexUsageResponse> {
  return requestJSON<CodexUsageResponse>('/v1/admin/llm/codex/usage')
}

export type LogsQuery = {
  file?: string
  level?: string
  component?: string
  lines?: number
}

export async function getLogs(query: LogsQuery = {}): Promise<LogsResponse> {
  const params = new URLSearchParams()
  if (query.file?.trim()) params.set('file', query.file.trim())
  if (query.level?.trim()) params.set('level', query.level.trim())
  if (query.component?.trim()) params.set('component', query.component.trim())
  if (query.lines && Number.isFinite(query.lines)) params.set('lines', String(query.lines))
  const suffix = params.toString()
  return requestJSON<LogsResponse>(`/v1/admin/logs${suffix ? `?${suffix}` : ''}`)
}

export async function getAnalytics(days = 7): Promise<AnalyticsResponse> {
  return requestJSON<AnalyticsResponse>(`/v1/admin/analytics?days=${days}`)
}

export async function getEventsHistory(limit = 30): Promise<EventsHistoryInfo> {
  return requestJSON<EventsHistoryInfo>(`/v1/events/history?limit=${limit}`)
}

export async function markEventsRead(lastId: number): Promise<{ unread_count: number }> {
  return requestJSON<{ acknowledged: boolean; read_cursor: number; unread_count: number }>('/v1/events/read', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ last_id: lastId }),
  })
}
