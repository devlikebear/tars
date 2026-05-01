import type { Session, SessionMessage } from './types'

export type ForkPreview = {
  message_id: string
  index: number
  role: string
  content: string
}

export type SessionLineageRow = {
  session: Session
  parent?: Session
  depth: number
  kind: 'root' | 'fork'
  branchLabel: string
  forkPreview?: ForkPreview
}

export function buildSessionLineageRows(
  sessions: Session[],
  previews: Record<string, ForkPreview | undefined> = {},
): SessionLineageRow[] {
  const byID = new Map<string, Session>()
  for (const session of sessions) {
    if (session.id?.trim()) byID.set(session.id, session)
  }

  const children = new Map<string, Session[]>()
  const roots: Session[] = []
  for (const session of sessions) {
    const parentID = session.parent_session_id?.trim()
    if (parentID && byID.has(parentID)) {
      const bucket = children.get(parentID) ?? []
      bucket.push(session)
      children.set(parentID, bucket)
    } else {
      roots.push(session)
    }
  }

  const sortSessions = (items: Session[]) => items.sort((a, b) => {
    const created = timestampValue(a.created_at) - timestampValue(b.created_at)
    if (created !== 0) return created
    return a.id.localeCompare(b.id)
  })

  sortSessions(roots)
  for (const bucket of children.values()) sortSessions(bucket)

  const rows: SessionLineageRow[] = []
  const seen = new Set<string>()
  const visit = (session: Session, depth: number, parent?: Session) => {
    if (seen.has(session.id)) return
    seen.add(session.id)
    rows.push({
      session,
      parent,
      depth,
      kind: parent ? 'fork' : 'root',
      branchLabel: depth === 0 ? '●' : '├─',
      forkPreview: previews[session.id],
    })
    for (const child of children.get(session.id) ?? []) {
      visit(child, depth + 1, session)
    }
  }

  for (const root of roots) visit(root, 0)
  for (const session of sortSessions(sessions.filter((item) => !seen.has(item.id)))) {
    visit(session, 0)
  }

  return rows
}

export function forkPreviewFromHistory(session: Session, history: SessionMessage[]): ForkPreview | undefined {
  if (!session.forked_from_message_id && session.forked_from_index === undefined) return undefined
  const messageID = session.forked_from_message_id?.trim() ?? ''
  let index = messageID ? history.findIndex((message) => message.id === messageID) : -1
  if (index < 0 && session.forked_from_index !== undefined) {
    index = session.forked_from_index
  }
  if (index < 0 || index >= history.length) return undefined
  const message = history[index]
  return {
    message_id: message.id,
    index,
    role: message.role,
    content: compactPreview(message.content),
  }
}

function compactPreview(value: string): string {
  const text = value.trim().replace(/\s+/g, ' ')
  if (text.length <= 160) return text
  return `${text.slice(0, 157).trim()}...`
}

function timestampValue(value: string): number {
  const time = Date.parse(value)
  return Number.isFinite(time) ? time : 0
}
