// Session board (#971): the console's home. Pure logic — status, grouping,
// filters, and the status changes worth a notification — so it is tested
// under Node without a DOM.

export type BoardServerStatus = 'needs_input' | 'running' | 'idle'

export type BoardChange = {
  turn_id: string
  at: string
  files: number
  additions: number
  deletions: number
}

// One row of GET /v1/chat/board.
export type BoardSession = {
  id: string
  title: string
  status: BoardServerStatus
  pending_approvals: number
  // Tool calls of unattended runs (cron, Telegram, subagents) waiting in the
  // ops queue (#970). The chat shows them too, so the card still opens it.
  queued_approvals?: number
  running_since?: string
  last_turn_at?: string
  updated_at: string
  pinned_at?: string
  cwd?: string
  repo?: string
  branch?: string
  last_change?: BoardChange
  cost_usd: number
  // Calls with no known price, left out of cost_usd. Absent before 0.45.6.
  unpriced_calls?: number
  goal?: { status?: string }
}

export type BoardResponse = {
  sessions: BoardSession[]
  cost_period: string
}

// What a card shows. `done_unread` is a finished turn nobody has looked at
// since; only this browser knows that, from `seen`.
export type BoardStatus = 'needs_input' | 'running' | 'done_unread' | 'idle'

export const boardStatusOrder: BoardStatus[] = ['needs_input', 'running', 'done_unread', 'idle']

// Seen maps a session ID to when it was last looked at (ms since epoch).
// The key '' holds when this browser started keeping track, which stands in
// for a session never opened here.
export type Seen = Record<string, number>

export const seenBaselineKey = ''

export function boardStatus(s: BoardSession, seen: Seen): BoardStatus {
  if (s.status === 'needs_input') return 'needs_input'
  if (s.status === 'running') return 'running'
  const last = Date.parse(s.last_turn_at ?? '')
  if (!Number.isFinite(last)) return 'idle'
  // Turns that ended before this browser kept track are not news.
  const seenAt = seen[s.id] ?? seen[seenBaselineKey]
  if (seenAt === undefined) return 'idle'
  return last > seenAt ? 'done_unread' : 'idle'
}

export type BoardFilter = 'all' | BoardStatus

export type BoardSort = 'recent' | 'status' | 'title' | 'cost'

export type BoardCard = BoardSession & { boardStatus: BoardStatus }

export type BoardGroup = {
  // Repository top level, or '' for sessions without a working folder.
  key: string
  // The repository's folder name; '' for the no-folder group.
  name: string
  cards: BoardCard[]
  counts: Record<BoardStatus, number>
}

export function repoName(path: string): string {
  const trimmed = path.replace(/[\\/]+$/, '')
  const parts = trimmed.split(/[\\/]/)
  return parts[parts.length - 1] || trimmed
}

function emptyCounts(): Record<BoardStatus, number> {
  return { needs_input: 0, running: 0, done_unread: 0, idle: 0 }
}

function matchesQuery(s: BoardSession, query: string): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  return [s.title, s.repo, s.branch, s.cwd].some((field) => (field ?? '').toLowerCase().includes(q))
}

function compareCards(sort: BoardSort) {
  return (a: BoardCard, b: BoardCard): number => {
    // Pinned sessions lead their group whatever the sort.
    const pinned = Number(Boolean(b.pinned_at)) - Number(Boolean(a.pinned_at))
    if (pinned !== 0) return pinned
    switch (sort) {
      case 'status': {
        const byStatus = boardStatusOrder.indexOf(a.boardStatus) - boardStatusOrder.indexOf(b.boardStatus)
        if (byStatus !== 0) return byStatus
        break
      }
      case 'title': {
        const byTitle = a.title.localeCompare(b.title)
        if (byTitle !== 0) return byTitle
        break
      }
      case 'cost': {
        if (b.cost_usd !== a.cost_usd) return b.cost_usd - a.cost_usd
        break
      }
    }
    return Date.parse(b.updated_at) - Date.parse(a.updated_at) || a.id.localeCompare(b.id)
  }
}

// groupBoard builds the board: repository groups, each with its cards and
// status counts. Groups that need someone come first, then the most recently
// active; the no-folder group always comes last.
export function groupBoard(
  sessions: BoardSession[],
  seen: Seen,
  options: { filter?: BoardFilter; sort?: BoardSort; query?: string } = {},
): BoardGroup[] {
  const filter = options.filter ?? 'all'
  const sort = options.sort ?? 'recent'
  const query = (options.query ?? '').trim()
  const groups = new Map<string, BoardGroup>()
  for (const s of sessions) {
    if (!matchesQuery(s, query)) continue
    const card: BoardCard = { ...s, boardStatus: boardStatus(s, seen) }
    if (filter !== 'all' && card.boardStatus !== filter) continue
    const key = s.repo ?? ''
    let group = groups.get(key)
    if (!group) {
      group = { key, name: key ? repoName(key) : '', cards: [], counts: emptyCounts() }
      groups.set(key, group)
    }
    group.cards.push(card)
    group.counts[card.boardStatus]++
  }
  const latest = (g: BoardGroup) => Math.max(...g.cards.map((c) => Date.parse(c.updated_at) || 0))
  const out = [...groups.values()]
  for (const g of out) g.cards.sort(compareCards(sort))
  out.sort((a, b) => {
    // Groups with a repository come before the one without.
    const aRepo = a.key !== ''
    if (aRepo !== (b.key !== '')) return aRepo ? -1 : 1
    const urgent = b.counts.needs_input - a.counts.needs_input
    if (urgent !== 0) return urgent
    return latest(b) - latest(a) || a.key.localeCompare(b.key)
  })
  return out
}

// Totals across every session for the filter chips, before filtering.
export function boardCounts(sessions: BoardSession[], seen: Seen): Record<BoardStatus, number> {
  const counts = emptyCounts()
  for (const s of sessions) counts[boardStatus(s, seen)]++
  return counts
}

// --- Notifications ---------------------------------------------------------

// pending counts the chat turn's approval cards; queued counts unattended
// runs' tool calls waiting in the ops queue (#970, #1033). Either one means
// the session needs input.
export type ActivityState = { running: boolean; pending: number; queued: number; title: string }

// Activity by session, from GET /v1/chat/activity.
export type ActivityMap = Record<string, ActivityState>

// An unattended run's tool call waiting in the ops queue. It is answered
// with POST /v1/ops/approvals/{approval_id}/approve|reject, never through
// the chat permission endpoint.
export type QueuedApproval = {
  approval_id: string
  session_id: string
  session_title?: string
  source?: string
  run_label?: string
  tool_name?: string
  preview?: string
  requested_at?: string
}

export type ActivitySnapshot = {
  running: { session_id: string; session_title?: string; started_at?: string }[]
  pending_approvals: { request_id: string; session_id: string; session_title?: string; tool_name?: string }[]
  // Absent from servers before #1033.
  queued_approvals?: QueuedApproval[]
}

export function activityMap(snap: ActivitySnapshot): ActivityMap {
  const out: ActivityMap = {}
  for (const r of snap.running ?? []) {
    out[r.session_id] = { running: true, pending: 0, queued: 0, title: r.session_title ?? '' }
  }
  for (const p of snap.pending_approvals ?? []) {
    const cur = out[p.session_id] ?? { running: true, pending: 0, queued: 0, title: p.session_title ?? '' }
    cur.pending++
    if (!cur.title) cur.title = p.session_title ?? ''
    out[p.session_id] = cur
  }
  // An unattended run is not a chat turn: it does not make the session
  // running, so its end is not announced as a finished turn.
  for (const q of snap.queued_approvals ?? []) {
    const cur = out[q.session_id] ?? { running: false, pending: 0, queued: 0, title: q.session_title ?? '' }
    cur.queued++
    if (!cur.title) cur.title = q.session_title ?? ''
    out[q.session_id] = cur
  }
  return out
}

// needsInput is whether someone has to answer something for the session:
// a chat approval card or an unattended approval.
export function needsInput(state: ActivityState | undefined): boolean {
  return Boolean(state && (state.pending > 0 || (state.queued ?? 0) > 0))
}

// needsInputHint says what a session waits for, e.g. for a tooltip:
// "1 approval waiting · 2 unattended approvals waiting".
export function needsInputHint(
  state: ActivityState | undefined,
  text: { pending: (n: number) => string; queued: (n: number) => string },
): string {
  if (!state) return ''
  const parts: string[] = []
  if (state.pending > 0) parts.push(text.pending(state.pending))
  if ((state.queued ?? 0) > 0) parts.push(text.queued(state.queued))
  return parts.join(' · ')
}

export type ActivityChange = { sessionId: string; title: string; kind: 'needs_input' | 'done' }

// activityChanges lists what happened between two polls that someone would
// want to hear about: a session started waiting for input, or a running
// session finished. A session still waiting stays quiet.
export function activityChanges(prev: ActivityMap, next: ActivityMap): ActivityChange[] {
  const out: ActivityChange[] = []
  for (const [id, cur] of Object.entries(next)) {
    const before = prev[id]
    if (needsInput(cur) && !needsInput(before)) {
      out.push({ sessionId: id, title: cur.title, kind: 'needs_input' })
    }
  }
  for (const [id, before] of Object.entries(prev)) {
    if (before.running && !next[id]) out.push({ sessionId: id, title: before.title, kind: 'done' })
  }
  return out
}

// --- Display ---------------------------------------------------------------

export type AgeLabels = {
  now: string
  seconds: (n: number) => string
  minutes: (n: number) => string
  hours: (n: number) => string
  days: (n: number) => string
}

// formatAge renders a duration in its largest whole unit: 45s, 12m, 3h, 2d.
export function formatAge(ms: number, labels: AgeLabels): string {
  if (!Number.isFinite(ms) || ms < 5_000) return labels.now
  const s = Math.floor(ms / 1000)
  if (s < 60) return labels.seconds(s)
  const m = Math.floor(s / 60)
  if (m < 60) return labels.minutes(m)
  const h = Math.floor(m / 60)
  if (h < 48) return labels.hours(h)
  return labels.days(Math.floor(h / 24))
}

// formatCost keeps two decimals, or three below a cent so a cheap session
// does not read as free.
export function formatCost(usd: number): string {
  if (!Number.isFinite(usd) || usd <= 0) return '0.00'
  return usd < 0.01 ? usd.toFixed(3) : usd.toFixed(2)
}
