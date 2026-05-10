import type { Session } from './types'

export type SessionKindFilter = 'all' | 'session' | 'main' | 'worker' | 'archived'
export type SessionSortMode = 'updated' | 'name'
export type SessionGroupKey = 'pinned' | 'recent' | 'older'

export type SessionGroup = {
  key: SessionGroupKey
  sessions: Session[]
}

export function sessionKind(session: Session): 'session' | 'main' | 'worker' {
  if (session.kind === 'main') return 'main'
  if (session.hidden) return 'worker'
  return 'session'
}

export function isArchived(session: Pick<Session, 'archived_at'>): boolean {
  return Boolean(session.archived_at?.trim())
}

export function isPinned(session: Pick<Session, 'pinned_at'>): boolean {
  return Boolean(session.pinned_at?.trim())
}

export function organizeSessions(
  sessions: Session[],
  options: {
    filterKind: SessionKindFilter
    sortBy: SessionSortMode
    query?: string
    hasTranscriptMatch?: (sessionID: string) => boolean
  },
): Session[] {
  const query = options.query?.trim().toLowerCase() ?? ''
  let result = sessions.filter((session) => {
    const matchesFilter = options.filterKind === 'archived'
      ? isArchived(session)
      : !isArchived(session) && (options.filterKind === 'all' || sessionKind(session) === options.filterKind)
    if (!matchesFilter) {
      return false
    }
    if (!query) {
      return true
    }
    return (session.title || '').toLowerCase().includes(query) ||
      session.id.toLowerCase().includes(query) ||
      Boolean(options.hasTranscriptMatch?.(session.id))
  })

  result = [...result].sort((a, b) => {
    const aPinned = isPinned(a)
    const bPinned = isPinned(b)
    if (aPinned !== bPinned) return aPinned ? -1 : 1
    if (aPinned && bPinned) {
      const pinDelta = timestamp(b.pinned_at) - timestamp(a.pinned_at)
      if (pinDelta !== 0) return pinDelta
    }
    if (options.sortBy === 'name') {
      return (a.title || a.id).localeCompare(b.title || b.id)
    }
    return timestamp(b.updated_at) - timestamp(a.updated_at)
  })

  return result
}

const RECENT_MS = 7 * 24 * 60 * 60 * 1000

export function groupSessions(
  organized: Session[],
  options: { useGroups: boolean },
  now = new Date(),
): SessionGroup[] {
  if (!options.useGroups || organized.length === 0) {
    return organized.length > 0 ? [{ key: 'older', sessions: organized }] : []
  }

  const nowMs = now.getTime()
  const pinned: Session[] = []
  const recent: Session[] = []
  const older: Session[] = []

  for (const session of organized) {
    if (isPinned(session)) {
      pinned.push(session)
    } else {
      const ts = timestamp(session.updated_at)
      if (ts > 0 && nowMs - ts < RECENT_MS) {
        recent.push(session)
      } else {
        older.push(session)
      }
    }
  }

  const groups: SessionGroup[] = []
  if (pinned.length > 0) groups.push({ key: 'pinned', sessions: pinned })
  if (recent.length > 0) groups.push({ key: 'recent', sessions: recent })
  if (older.length > 0) groups.push({ key: 'older', sessions: older })
  return groups
}

export function cleanupCandidateSessions(sessions: Session[], now = new Date(), limit = 8): Session[] {
  return organizeSessions(sessions, { filterKind: 'all', sortBy: 'updated' })
    .filter((session) => {
      if (sessionKind(session) !== 'session' || isPinned(session) || isArchived(session)) return false
      const title = (session.title || '').trim().toLowerCase()
      const ageDays = Math.max(0, (now.getTime() - timestamp(session.updated_at)) / 86400000)
      const genericTitle = title === '' ||
        title === 'new chat' ||
        title === 'untitled' ||
        title === 'chat' ||
        title === '새 대화'
      const tinyStaleTitle = title.length > 0 && title.length <= 4 && ageDays >= 1
      return (genericTitle && ageDays >= 1) || tinyStaleTitle || ageDays >= 14
    })
    .slice(0, limit)
}

function timestamp(value?: string): number {
  const parsed = Date.parse(value || '')
  return Number.isNaN(parsed) ? 0 : parsed
}
