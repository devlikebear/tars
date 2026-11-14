// The phone's "waiting on you" inbox (docs/decisions/mobile-remote.md §2):
// one list across every paired desktop of the focus pipelines that wait on
// their developer, each labeled with the desktop that owns it. Nothing here
// decides what waits: each desktop says so in its pipeline list (the rule
// its Telegram alert uses) and in its chat activity (tool calls waiting for
// an answer).
import type { FocusGate, FocusListItem } from '../types.ts'
import type { ActivitySnapshot } from '../sessionBoard.ts'

// A tool call waiting for an answer in one of a pipeline's sessions.
//   queued     — an unattended turn's call in the desk's ops queue
//                (POST /v1/ops/approvals/{id}/approve|reject)
//   permission — a turn's question asked while the desk's console is open
//                (POST /v1/chat/permissions/{id})
export type InboxApproval = {
  kind: 'queued' | 'permission'
  id: string
  sessionID: string
  toolName: string
  preview: string
  at: string
}

export type InboxEntry = {
  // desktopID + session: unique across desktops.
  key: string
  desktopID: string
  desktopName: string
  sessionID: string
  title: string
  current: string
  currentLabel: string
  template: string
  gate: FocusGate
  cards: number
  approvals: InboxApproval[]
  updatedAt: string
}

// What one desktop answered, or why it did not.
export type DesktopInbox = {
  desktopID: string
  desktopName: string
  pipelines: FocusListItem[]
  activity: ActivitySnapshot
}

// waitingOf reads what a pipeline waits on. A server from before the phone
// inbox sends no `waiting`: its open gate and unseen cards stand in, with
// goal mode waiting on nobody, as the server's own rule has it.
export function waitingOf(item: FocusListItem): { gate: FocusGate; cards: number } {
  if (item.waiting) return { gate: item.waiting.gate ?? '', cards: item.waiting.cards ?? 0 }
  if (item.goal_mode) return { gate: '', cards: 0 }
  const gate = item.open_gate ?? ''
  // needs_input counts the open gate's own card too.
  const cards = Math.max(0, (item.needs_input ?? 0) - (gate ? 1 : 0))
  return { gate, cards }
}

// inboxOf builds one desktop's entries: every pipeline something waits on,
// with the tool calls waiting in its session or its Q&A session. A call in
// a session the list does not name (the list moved on) still shows, under
// that session's title, so a question is never hidden.
export function inboxOf(desk: DesktopInbox): InboxEntry[] {
  const bySession = new Map<string, FocusListItem>()
  for (const item of desk.pipelines) {
    bySession.set(item.session_id, item)
    if (item.qa_session_id) bySession.set(item.qa_session_id, item)
  }
  const entries = new Map<string, InboxEntry>()
  const entryFor = (sessionID: string, title: string, item?: FocusListItem): InboxEntry => {
    const id = item?.session_id ?? sessionID
    let entry = entries.get(id)
    if (!entry) {
      const waiting = item ? waitingOf(item) : { gate: '' as FocusGate, cards: 0 }
      entry = {
        key: `${desk.desktopID}\n${id}`,
        desktopID: desk.desktopID,
        desktopName: desk.desktopName,
        sessionID: id,
        title: item?.title || item?.goal || title || id,
        current: item?.current ?? '',
        currentLabel: item?.current_label ?? '',
        template: item?.template ?? '',
        gate: waiting.gate,
        cards: waiting.cards,
        approvals: [],
        updatedAt: item?.updated_at ?? '',
      }
      entries.set(id, entry)
    }
    return entry
  }

  for (const item of desk.pipelines) {
    const w = waitingOf(item)
    if (w.gate || w.cards > 0) entryFor(item.session_id, item.title, item)
  }
  for (const q of desk.activity.queued_approvals ?? []) {
    const entry = entryFor(q.session_id, q.session_title ?? '', bySession.get(q.session_id))
    entry.approvals.push({
      kind: 'queued',
      id: q.approval_id,
      sessionID: q.session_id,
      toolName: q.tool_name ?? '',
      preview: q.preview ?? '',
      at: q.requested_at ?? '',
    })
    entry.updatedAt = later(entry.updatedAt, q.requested_at)
  }
  for (const p of desk.activity.pending_approvals ?? []) {
    const entry = entryFor(p.session_id, p.session_title ?? '', bySession.get(p.session_id))
    entry.approvals.push({
      kind: 'permission',
      id: p.request_id,
      sessionID: p.session_id,
      toolName: p.tool_name ?? '',
      preview: p.preview ?? '',
      at: p.asked_at ?? '',
    })
    entry.updatedAt = later(entry.updatedAt, p.asked_at)
  }
  return [...entries.values()]
}

function timeOf(at: string | undefined): number {
  const ms = at ? Date.parse(at) : NaN
  return Number.isNaN(ms) ? 0 : ms
}

function later(a: string, b: string | undefined): string {
  return b && timeOf(b) > timeOf(a) ? b : a
}

// mergeInboxes puts every desktop's entries in one list: tool calls first
// (a turn is stopped on them), then the most recently changed.
export function mergeInboxes(desks: DesktopInbox[]): InboxEntry[] {
  const all = desks.flatMap(inboxOf)
  return all.sort((a, b) => {
    const calls = Number(b.approvals.length > 0) - Number(a.approvals.length > 0)
    if (calls) return calls
    return timeOf(b.updatedAt) - timeOf(a.updatedAt)
  })
}

// consoleURL is where a pipeline opens in its desktop's console.
export function consoleURL(baseURL: string, sessionID: string): string {
  return `${baseURL.replace(/\/+$/, '')}/console/focus/${encodeURIComponent(sessionID)}`
}
