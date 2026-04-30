import type { PulseSnapshot, ReflectionSnapshot, Session } from './types'

export type StatusTone = 'ok' | 'warn' | 'error'

export function formatRelativeStatusTime(value?: string, now = new Date()): string {
  const text = value?.trim()
  if (!text) return 'never'

  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return text
  if (date.getFullYear() <= 1) return 'never'

  const seconds = Math.max(0, Math.floor((now.getTime() - date.getTime()) / 1000))
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

export function derivePulseTone(snapshot: Partial<PulseSnapshot> | null): StatusTone {
  if (!snapshot) return 'warn'
  if (snapshot.last_err?.trim()) return 'error'
  if (!hasUsableTimestamp(snapshot.last_tick_at)) return 'warn'
  return 'ok'
}

export function deriveReflectionTone(snapshot: Partial<ReflectionSnapshot> | null): StatusTone {
  if (!snapshot) return 'warn'
  if ((snapshot.consecutive_failures ?? 0) > 0 || snapshot.last_run_success === false) return 'error'
  if (!hasUsableTimestamp(snapshot.last_run_at) && !hasUsableTimestamp(snapshot.last_successful_run_at)) return 'warn'
  return 'ok'
}

export function countActiveSessions(sessions: Pick<Session, 'hidden'>[]): number {
  return sessions.filter((session) => !session.hidden).length
}

function hasUsableTimestamp(value?: string): boolean {
  const text = value?.trim()
  if (!text) return false
  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return false
  return date.getFullYear() > 1
}
