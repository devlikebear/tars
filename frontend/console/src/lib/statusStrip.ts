import type { ChatThreadTranslations } from '../i18n/sections/chatThread'
import type { PulseSnapshot, ReflectionSnapshot, Session } from './types'

export type StatusTone = 'ok' | 'warn' | 'error'

// Pass $t.chatThread.statusStrip.relativeTime.
export type RelativeStatusTimeLabels = ChatThreadTranslations['statusStrip']['relativeTime']

export function formatRelativeStatusTime(value: string | undefined, labels: RelativeStatusTimeLabels, now = new Date()): string {
  const text = value?.trim()
  if (!text) return labels.never

  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return text
  if (date.getFullYear() <= 1) return labels.never

  const seconds = Math.max(0, Math.floor((now.getTime() - date.getTime()) / 1000))
  if (seconds < 60) return labels.secondsAgo(seconds)
  if (seconds < 3600) return labels.minutesAgo(Math.floor(seconds / 60))
  if (seconds < 86400) return labels.hoursAgo(Math.floor(seconds / 3600))
  return labels.daysAgo(Math.floor(seconds / 86400))
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
