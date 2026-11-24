// The vibe-mode home and project board (docs/plans/vibe-mode-redesign, unit
// 2). Every project's state is the server's (internal/easyproject board.go
// BoardOf, listed by GET /v1/easy/projects): nothing here decides where a
// project stands. This file only places the server's states in the board's
// columns, merges the pages it reads, and words the card's one line.

import { easyBase, type EasyProject } from './easy.ts'

// A project's board state, as the server gives it.
export type EasyBoardState = 'starting' | 'failed' | 'your_turn' | 'working' | 'paused' | 'stuck' | 'done' | 'published'

// The facts a card shows (easyproject.Board).
export type EasyBoard = {
  state: EasyBoardState
  waiting?: 'plan' | 'question' | string
  resets?: string
  resets_at?: string
  stuck?: 'stopped' | 'goal_ended' | 'blocked' | string
  check?: { screen?: string; expected?: string; seen?: string }
  save_failed?: boolean
  changed_at: string
}

export type EasyListed = EasyProject & { board: EasyBoard; archived_at?: string }

// One page of GET /v1/easy/projects.
export type EasyListPage = {
  projects: EasyListed[]
  next_cursor?: string
  counts: Partial<Record<EasyBoardState, number>>
  archived: number
}

export type EasyListQuery = { state?: EasyBoardState; limit?: number; cursor?: string; archived?: boolean }

// The query string of a list request.
export function listQueryString(q: EasyListQuery): string {
  const params = new URLSearchParams()
  if (q.state) params.set('state', q.state)
  if (q.limit) params.set('limit', String(q.limit))
  if (q.cursor) params.set('cursor', q.cursor)
  if (q.archived) params.set('archived', '1')
  const s = params.toString()
  return s ? `?${s}` : ''
}

// The board's five columns and the server states each one holds, in the
// order the design shows them. A project being set up is already the
// team's work; one that could not start has stopped like a stuck one.
export const BOARD_COLUMNS = ['your_turn', 'working', 'stopped', 'done', 'published'] as const
export type EasyColumn = (typeof BOARD_COLUMNS)[number]

export const COLUMN_STATES: Record<EasyColumn, EasyBoardState[]> = {
  your_turn: ['your_turn'],
  working: ['working', 'starting'],
  stopped: ['paused', 'stuck', 'failed'],
  done: ['done'],
  published: ['published'],
}

export function columnOf(state: EasyBoardState): EasyColumn {
  for (const column of BOARD_COLUMNS) if (COLUMN_STATES[column].includes(state)) return column
  return 'working'
}

// How many projects a column holds, from the server's counts.
export function columnCount(column: EasyColumn, counts: EasyListPage['counts']): number {
  return COLUMN_STATES[column].reduce((sum, s) => sum + (counts[s] ?? 0), 0)
}

// Every project on the board (archived ones are not).
export function boardTotal(counts: EasyListPage['counts']): number {
  return Object.values(counts).reduce((sum: number, n) => sum + (n ?? 0), 0)
}

// The board's page size: five cards a column, then "more".
export const COLUMN_PAGE = 5

// The board's order, the server's own (easyproject.before): newest change
// first, then the larger id.
export function boardBefore(a: EasyListed, b: EasyListed): boolean {
  const ta = Date.parse(a.board.changed_at)
  const tb = Date.parse(b.board.changed_at)
  if (ta !== tb) return ta > tb
  return a.id > b.id
}

function sortBoard(items: EasyListed[]): EasyListed[] {
  return [...items].sort((a, b) => (boardBefore(a, b) ? -1 : boardBefore(b, a) ? 1 : 0))
}

// A column reads each of its states as its own paged list (the server
// filters one state at a time) and shows them merged. A state's list is
// what was read so far and the cursor of its next page.
export type StateFeed = { items: EasyListed[]; next?: string; read: boolean }
export type ColumnFeed = { feeds: Partial<Record<EasyBoardState, StateFeed>>; shown: number }

export function emptyColumn(column: EasyColumn): ColumnFeed {
  const feeds: ColumnFeed['feeds'] = {}
  for (const s of COLUMN_STATES[column]) feeds[s] = { items: [], read: false }
  return { feeds, shown: COLUMN_PAGE }
}

function merged(feed: ColumnFeed): EasyListed[] {
  return sortBoard(Object.values(feed.feeds).flatMap((f) => f?.items ?? []))
}

// The states whose next page must be read before the column can show its
// first `shown` cards in the right order: one never read, or one with more
// pages whose last card read would still be among them.
export function columnNeeds(feed: ColumnFeed): EasyBoardState[] {
  const all = merged(feed)
  const cutoff = all.length >= feed.shown ? all[feed.shown - 1] : undefined
  const needs: EasyBoardState[] = []
  for (const [state, f] of Object.entries(feed.feeds) as Array<[EasyBoardState, StateFeed]>) {
    if (!f.read) {
      needs.push(state)
      continue
    }
    if (!f.next) continue
    const last = f.items[f.items.length - 1]
    if (!cutoff || !last || boardBefore(last, cutoff)) needs.push(state)
  }
  return needs
}

// A page read for one of a column's states.
export function withPage(feed: ColumnFeed, state: EasyBoardState, page: Pick<EasyListPage, 'projects' | 'next_cursor'>): ColumnFeed {
  const prev = feed.feeds[state] ?? { items: [], read: false }
  const seen = new Set(prev.items.map((p) => p.id))
  const items = [...prev.items, ...page.projects.filter((p) => !seen.has(p.id))]
  return { ...feed, feeds: { ...feed.feeds, [state]: { items, next: page.next_cursor || undefined, read: true } } }
}

// What the column shows, and whether "more" has anything behind it.
export function columnView(feed: ColumnFeed): { cards: EasyListed[]; more: boolean } {
  const all = merged(feed)
  const more = all.length > feed.shown || Object.values(feed.feeds).some((f) => f?.next)
  return { cards: all.slice(0, feed.shown), more }
}

export function showMore(feed: ColumnFeed): ColumnFeed {
  return { ...feed, shown: feed.shown + COLUMN_PAGE }
}

// A card gone from the board (archived, deleted) or renamed in place.
export function withoutProject(feed: ColumnFeed, id: string): ColumnFeed {
  const feeds: ColumnFeed['feeds'] = {}
  for (const [state, f] of Object.entries(feed.feeds) as Array<[EasyBoardState, StateFeed]>) {
    feeds[state] = { ...f, items: f.items.filter((p) => p.id !== id) }
  }
  return { ...feed, feeds }
}

export function renamedIn(feed: ColumnFeed, id: string, name: string): ColumnFeed {
  const feeds: ColumnFeed['feeds'] = {}
  for (const [state, f] of Object.entries(feed.feeds) as Array<[EasyBoardState, StateFeed]>) {
    feeds[state] = { ...f, items: f.items.map((p) => (p.id === id ? { ...p, name } : p)) }
  }
  return { ...feed, feeds }
}

// Searching by name reads every project once and keeps the ones whose name
// holds the words, ignoring case.
export function matchesName(project: Pick<EasyProject, 'name'>, query: string): boolean {
  const q = query.trim().toLocaleLowerCase()
  return !q || project.name.toLocaleLowerCase().includes(q)
}

export function groupByColumn(projects: EasyListed[]): Record<EasyColumn, EasyListed[]> {
  const out = Object.fromEntries(BOARD_COLUMNS.map((c) => [c, [] as EasyListed[]])) as Record<EasyColumn, EasyListed[]>
  for (const p of sortBoard(projects)) out[columnOf(p.board.state)].push(p)
  return out
}

// The home screen's three recent projects: one waiting on the person
// first, then the newest others.
export const HOME_RECENT = 3

export function homeRecent(recent: EasyListed[], waiting: EasyListed | undefined, max = HOME_RECENT): EasyListed[] {
  const out = waiting ? [waiting] : []
  for (const p of recent) {
    if (out.length >= max) break
    if (!out.some((o) => o.id === p.id)) out.push(p)
  }
  return out
}

// What a card's line says, as a key into the strings, with what it needs.
export type CardLine =
  | { key: 'plan' | 'question' | 'waiting' | 'working' | 'starting' | 'failed' | 'stopped' | 'stuck' | 'saveFailed' | 'pausedSoon' | 'published' }
  | { key: 'paused'; when: string }
  | { key: 'done'; ago: string }

// The tone a line is drawn in: the person's turn, the team at work, a
// problem, or nothing in particular.
export type CardTone = 'turn' | 'work' | 'problem' | 'quiet'

export function cardLine(board: EasyBoard, locale: string, now = new Date()): { line: CardLine; tone: CardTone } {
  switch (board.state) {
    case 'your_turn':
      return { line: { key: board.waiting === 'plan' ? 'plan' : board.waiting === 'question' ? 'question' : 'waiting' }, tone: 'turn' }
    case 'working':
      return { line: { key: 'working' }, tone: 'work' }
    case 'starting':
      return { line: { key: 'starting' }, tone: 'work' }
    case 'failed':
      return { line: { key: 'failed' }, tone: 'problem' }
    case 'paused': {
      const when = resetsWhen(board, locale, now)
      return { line: when ? { key: 'paused', when } : { key: 'pausedSoon' }, tone: 'quiet' }
    }
    case 'stuck':
      return { line: { key: board.stuck === 'stopped' ? 'stopped' : 'stuck' }, tone: board.stuck === 'stopped' ? 'quiet' : 'problem' }
    case 'done':
      if (board.save_failed) return { line: { key: 'saveFailed' }, tone: 'problem' }
      return { line: { key: 'done', ago: easyAgo(board.changed_at, locale, now) }, tone: 'quiet' }
    case 'published':
      return { line: { key: 'published' }, tone: 'quiet' }
  }
  return { line: { key: 'working' }, tone: 'work' }
}

// When a paused project's limit lifts: a local time (with the day when it
// is not today), else the provider's own words.
function resetsWhen(board: EasyBoard, locale: string, now: Date): string {
  if (board.resets_at) {
    const at = new Date(board.resets_at)
    if (!Number.isNaN(at.getTime())) {
      const sameDay = at.toDateString() === now.toDateString()
      return at.toLocaleString(locale, sameDay ? { hour: 'numeric', minute: '2-digit' } : { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
    }
  }
  return board.resets ?? ''
}

// How long ago, in the person's language ("10 minutes ago", "yesterday").
export function easyAgo(at: string, locale: string, now = new Date()): string {
  const then = Date.parse(at)
  if (Number.isNaN(then)) return ''
  const seconds = Math.round((then - now.getTime()) / 1000)
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const abs = Math.abs(seconds)
  if (abs < 60) return rtf.format(0, 'second')
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), 'minute')
  if (abs < 86_400) return rtf.format(Math.round(seconds / 3600), 'hour')
  if (abs < 7 * 86_400) return rtf.format(Math.round(seconds / 86_400), 'day')
  if (abs < 30 * 86_400) return rtf.format(Math.round(seconds / (7 * 86_400)), 'week')
  return new Date(then).toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric' })
}

// The answer to a rename, archive or delete the server refused, as a key
// into the strings: the team is at work (409 busy), the name is empty or
// too long, the folder is not where the projects are, or anything else.
export function manageErrorKey(code: string): 'busy' | 'nameRequired' | 'nameTooLong' | 'folderOutside' | 'notFound' | 'unknown' {
  switch (code) {
    case 'busy':
      return 'busy'
    case 'name_required':
      return 'nameRequired'
    case 'name_too_long':
      return 'nameTooLong'
    case 'folder_outside':
      return 'folderOutside'
    case 'not_found':
      return 'notFound'
    default:
      return 'unknown'
  }
}

// Whether the board lets the person try to archive or delete a project:
// not while the team is at it. The server decides (409 busy); this only
// keeps a click that would be refused from being offered.
export function manageable(state: EasyBoardState): boolean {
  return state !== 'working' && state !== 'starting'
}

// The vibe shell's routes.
export const easyProjectsPath = `${easyBase}/projects`
export const easyArchivePath = `${easyBase}/archive`

// How often the shell reads the count of projects waiting on the person and
// the AI's readiness.
export const shellPollMs = 15_000
