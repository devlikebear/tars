// Easy mode (docs/decisions/vibe-coder-identity.md §4–§5): what a person who
// cannot read code sees of a project — a team of bots doing the stages in
// turn and, at most, one thing for them to do. Everything here reads the
// project record (POST /v1/easy/projects) and its focus pipeline; nothing
// decides a transition. The screens only render what easyView answers.

import type { FocusCard, FocusPipeline, FocusStage } from './types'

export type EasyProjectStatus = 'creating' | 'ready' | 'failed'

// An easy-mode project as the server records it (internal/easyproject).
export type EasyProject = {
  id: string
  name: string
  idea: string
  folder: string
  status: EasyProjectStatus
  error_code?: string
  session_id?: string
  created_at: string
  updated_at: string
}

// The team, in the order it works. The publisher arrives with phase 5.
export const EASY_BOTS = ['leader', 'maker', 'reviewer', 'checker', 'publisher'] as const
export type EasyBot = (typeof EASY_BOTS)[number]

// What a bot is doing: not its turn yet, at work, finished, waiting on the
// person, or not part of this version.
export type EasyBotState = 'waiting' | 'working' | 'done' | 'needs_you' | 'later'

// Where the project is, as one word the screen turns into a sentence.
export type EasyPhase =
  | 'creating' // the folder and stack are being set up
  | 'failed' // creation failed (errorCode says why)
  | 'planning' // the leader is writing the plan
  | 'approve' // the plan waits for the person
  | 'building' // the team is making, reviewing and checking it
  | 'stuck' // something stopped the team and needs the person
  | 'finished' // done and saved

export type EasyCheckFailure = { screen?: string; expected: string; seen: string }

export type EasyView = {
  phase: EasyPhase
  bots: Record<EasyBot, EasyBotState>
  errorCode?: string
  // The plan to approve, in the leader's own words.
  plan?: { goal: string; tasks: string[] }
  // The last failed check, in the check's own words (scripted browser
  // checks print screen / expected / seen; the server copies them).
  check?: EasyCheckFailure
  // Whether the finished version was saved (Pipeline.saved).
  saved?: boolean
}

// The stage kinds a bot works in.
const botOfKind: Record<string, EasyBot> = { plan: 'leader', build: 'maker', review: 'reviewer' }

function kindOf(stage: FocusStage): string {
  return stage.kind ?? stage.id
}

function stageState(stage: FocusStage | undefined): EasyBotState {
  switch (stage?.status) {
    case 'done':
    case 'skipped':
      return 'done'
    case 'active':
      return 'working'
    case 'blocked':
      return 'needs_you'
    default:
      return 'waiting'
  }
}

function lastCheckFailure(cards: FocusCard[]): EasyCheckFailure | undefined {
  for (let i = cards.length - 1; i >= 0; i--) {
    const card = cards[i]
    if (card.kind !== 'failure') continue
    const check = (card.payload as { check?: EasyCheckFailure } | undefined)?.check
    if (check?.expected && check.seen) return { screen: check.screen, expected: check.expected, seen: check.seen }
    return undefined
  }
  return undefined
}

// A pipeline whose save is recorded; the field is new (2-3), so it is read
// loosely.
type SavedField = { saved?: { commit?: string; error?: string } }

export function easyView(project: EasyProject, pipeline: (FocusPipeline & SavedField) | null | undefined): EasyView {
  const bots: Record<EasyBot, EasyBotState> = { leader: 'waiting', maker: 'waiting', reviewer: 'waiting', checker: 'waiting', publisher: 'later' }
  if (project.status === 'failed') return { phase: 'failed', bots, errorCode: project.error_code || 'unknown' }
  if (project.status === 'creating' || !pipeline) {
    bots.leader = 'working'
    return { phase: 'creating', bots }
  }

  for (const stage of pipeline.stages) {
    const bot = botOfKind[kindOf(stage)]
    if (bot) bots[bot] = stageState(stage)
  }
  // The checker runs the checks at the end of every making and reviewing
  // round; it is at work while the server runs them.
  if (pipeline.awaiting_verification) bots.checker = 'working'

  const view: EasyView = { phase: 'building', bots, check: lastCheckFailure(pipeline.cards) }
  if (pipeline.finished_at) {
    bots.checker = 'done'
    view.phase = 'finished'
    view.saved = Boolean(pipeline.saved?.commit) && !pipeline.saved?.error
    return view
  }
  if (pipeline.open_gate === 'plan' && pipeline.plan) {
    bots.leader = 'needs_you'
    view.phase = 'approve'
    view.plan = { goal: pipeline.plan.goal, tasks: pipeline.plan.tasks.map((t) => t.title) }
    return view
  }
  const blocked = pipeline.stages.some((s) => s.status === 'blocked')
  const goalGone = pipeline.goal_mode && !pipeline.goal_mode.enabled && pipeline.goal_mode.end_reason !== 'finished'
  if (blocked || pipeline.open_gate === 'blocked' || goalGone) {
    view.phase = 'stuck'
    return view
  }
  if (pipeline.current === 'plan' || kindOf(pipeline.stages.find((s) => s.id === pipeline.current) ?? ({ id: 'plan' } as FocusStage)) === 'plan') {
    view.phase = 'planning'
  }
  return view
}

// Whether a project still changes on its own, so the screen keeps reading.
export function easyLive(view: EasyView): boolean {
  return view.phase === 'creating' || view.phase === 'planning' || view.phase === 'building'
}

// Words an easy-mode screen never shows (vibe-coder-identity.md §4): the
// machinery under the screen. Matched as whole words, ignoring case.
export const EASY_HIDDEN_WORDS = [
  'git', 'commit', 'branch', 'worktree', 'pull request', 'PR', 'merge', 'tier',
  'permission mode', 'verify', 'triage', 'gate', 'pipeline',
  '커밋', '브랜치', '워크트리', '머지', '티어', '권한 모드', '트리아지', '게이트', '파이프라인',
]

// The hidden words found in text, for the screen check.
export function hiddenWordsIn(text: string): string[] {
  const found: string[] = []
  for (const word of EASY_HIDDEN_WORDS) {
    const escaped = word.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    // Hangul has no word boundary in \b; a Korean word matches anywhere.
    const pattern = /^[\x00-\x7f]+$/.test(word) ? new RegExp(`\\b${escaped}\\b`, word === 'PR' ? '' : 'i') : new RegExp(escaped)
    if (pattern.test(text)) found.push(word)
  }
  return found
}

// The routes of easy mode.
export const easyBase = '/console/easy'
export function easyProjectPath(id: string): string {
  return `${easyBase}/${encodeURIComponent(id)}`
}

// The result screen's preview (internal/easyproject/preview.go): the
// person's app, run by the server on this computer's loopback address.
export type EasyPreviewState = 'stopped' | 'starting' | 'running' | 'failed'
export type EasyPreview = { state: EasyPreviewState; url?: string; error_code?: string }

// When the result screen reads its preview again: soon while it starts,
// every half minute while it runs (reading keeps it running; the server
// stops one nobody read for ten minutes), never once it stopped or failed.
export function previewPollMs(preview: EasyPreview): number | null {
  switch (preview.state) {
    case 'starting':
      return 1000
    case 'running':
      return 30_000
    default:
      return null
  }
}

// Whether a console at hostname can show the preview: the app answers only
// on the server's own computer, so a console reached from another one (a
// remote address) cannot load it.
export function previewReachable(hostname: string): boolean {
  const host = hostname.replace(/^\[|\]$/g, '')
  return host === 'localhost' || host === '::1' || host.endsWith('.localhost') || /^127\./.test(host)
}
