// Easy mode (docs/decisions/vibe-coder-identity.md §4–§5): what a person who
// cannot read code sees of a project — a team of bots doing the stages in
// turn and, at most, one thing for them to do. Everything here reads the
// project record (POST /v1/easy/projects) and its focus pipeline; nothing
// decides a transition. The screens only render what easyView answers.

import type { FocusBlocked, FocusCard, FocusCheckResult, FocusDecision, FocusPipeline, FocusPlan, FocusStage } from './types'

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
  // What the person asked to change after the first version, oldest first.
  changes?: EasyChange[]
  created_at: string
  updated_at: string
}

// One "change it like this" and the pipeline that makes it.
export type EasyChange = { request: string; session_id: string; at: string }

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
  | 'questions' // the leader asks the person a few questions before the plan
  | 'approve' // the plan waits for the person
  | 'building' // the team is making, reviewing and checking it
  | 'stuck' // something stopped the team and needs the person
  | 'limit' // the AI's usage limit is spent; the team goes on when it is back
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
  // Whether the finished version was saved (Pipeline.saved); undefined
  // while the server is still saving it.
  saved?: boolean
  // The person's own words, when this run makes a change they asked for
  // ("change it like this") rather than the first version.
  change?: string
  // When the AI's usage limit lifts (phase 'limit'), and the card the
  // "Continue" answer goes to.
  limit?: EasyLimit
  // The workspace (docs/plans/vibe-mode-redesign unit 4): which of the four
  // steps the project is in, the bot at work, the leader's questions, the
  // blueprint, the checks with their results and the team's log.
  step: EasyStep
  steps: Record<EasyStep, EasyStepState>
  working?: EasyBot
  questions: EasyQuestion[]
  blueprint?: EasyBlueprint
  checks: EasyCheckRow[]
  log: EasyLogEntry[]
}

// The four steps of the workspace header. Publishing comes with a later
// phase; until then it is never the step a project is in.
export const EASY_STEPS = ['plan', 'make', 'check', 'publish'] as const
export type EasyStep = (typeof EASY_STEPS)[number]
export type EasyStepState = 'done' | 'now' | 'next'

// The answer of a question's "you decide" choice
// (focuspipeline.DecisionYouDecide): the leader reads it as "leave it to
// you". The screen shows it in the person's language.
export const EASY_YOU_DECIDE = 'You decide'

// One of the leader's questions before the plan (a decision card): its
// choices, whether the server offers "you decide", and the answer once
// given.
export type EasyQuestion = { cardId: string; question: string; options: string[]; youDecide: boolean; answer?: string }

// A rough sketch of one screen of the blueprint: its parts are a fixed
// vocabulary the console draws as simple shapes.
export const EASY_SCREEN_PARTS = ['heading', 'text', 'field', 'button', 'list', 'image', 'link'] as const
export type EasyScreenPartKind = (typeof EASY_SCREEN_PARTS)[number]
export type EasyScreen = { name: string; line: string; parts: { kind: EasyScreenPartKind; text: string }[] }

// The plan as the person reads it. A plan without blueprint fields (a
// project from before them) has only its goal and tasks.
export type EasyBlueprint = {
  goal: string
  tasks: string[]
  screens: EasyScreen[]
  checks: string[]
  saved: string[]
  notNow: string[]
}

// A check of the blueprint with the server's latest result. The console
// never judges a check: the state and the report are the server's.
export type EasyCheckResult = FocusCheckResult['state']
export type EasyCheckRow = { label: string; state: EasyCheckResult; timedOut?: boolean; report?: EasyCheckFailure }

// One line of the team's log, built from facts the server keeps (cards,
// stages, results), never from the model's own words: who says it and
// which fixed sentence. The person's own words come first.
export type EasyLogLine =
  | 'idea' // the person's idea (text)
  | 'change' // what the person asked to change (text)
  | 'asked' // the leader asks its questions (they follow this line)
  | 'drawing' // the leader is drawing the blueprint
  | 'drew' // the blueprint is ready for the person
  | 'handed' // the approved blueprint went to the maker
  | 'making' // the maker is making it
  | 'made' // the maker finished making it
  | 'reviewing' // the reviewer looks for problems
  | 'fixing' // the maker fixes what the reviewer found
  | 'checked' // the checker ran the checks (passed of total)
  | 'fixingCheck' // a check failed; the maker is fixing it
  | 'finished' // all done
export type EasyLogEntry = { who: EasyBot | 'you'; line: EasyLogLine; text?: string; passed?: number; total?: number }

// A spent usage limit (FocusBlocked.usage_limit): resets is the provider's
// own words for when it comes back, resetsAt a time when it gave one.
export type EasyLimit = { cardId: string; resets?: string; resetsAt?: string }

// The usage limit the pipeline stopped on: the open blocked gate's card,
// when the server recorded the failed turn as refused by the limit.
function usageLimitStop(pipeline: FocusPipeline): EasyLimit | undefined {
  if (pipeline.open_gate !== 'blocked') return undefined
  const card = pipeline.cards.findLast((c) => c.kind === 'gate' && c.state !== 'decided')
  const fact = card?.payload as FocusBlocked | undefined
  if (!card || fact?.reason !== 'turn_failed' || !fact.usage_limit) return undefined
  return { cardId: card.id, resets: fact.resets || undefined, resetsAt: fact.resets_at || undefined }
}

// When the limit comes back, as the screen says it: the provider's time as
// a local clock time (with the day when it is not today), else the
// provider's own words, else nothing.
export function easyLimitResets(limit: EasyLimit, locale: string, now = new Date()): string {
  if (limit.resetsAt) {
    const at = new Date(limit.resetsAt)
    if (!Number.isNaN(at.getTime())) {
      const sameDay = at.toDateString() === now.toDateString()
      return at.toLocaleString(locale, sameDay ? { hour: 'numeric', minute: '2-digit' } : { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
    }
  }
  return limit.resets ?? ''
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
  const view = phaseView(project, pipeline)
  return { ...view, ...workspace(project, pipeline, view) }
}

type PhaseView = Omit<EasyView, 'step' | 'steps' | 'working' | 'questions' | 'blueprint' | 'checks' | 'log'>

function phaseView(project: EasyProject, pipeline: (FocusPipeline & SavedField) | null | undefined): PhaseView {
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

  const view: PhaseView = { phase: 'building', bots, check: lastCheckFailure(pipeline.cards) }
  const change = project.changes?.findLast((c) => c.session_id === project.session_id)
  if (change) view.change = change.request
  if (pipeline.finished_at) {
    bots.checker = 'done'
    view.phase = 'finished'
    // The server saves the version a moment after the pipeline finishes.
    if (pipeline.saved) view.saved = Boolean(pipeline.saved.commit) && !pipeline.saved.error
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
    // Nothing is wrong with the work when the usage limit stopped it: the
    // person waits and says when to go on (goal mode comes back with it).
    const limit = usageLimitStop(pipeline)
    view.phase = limit ? 'limit' : 'stuck'
    if (limit) view.limit = limit
    return view
  }
  // The leader's questions before the plan wait for the person's answers.
  if (openQuestions(pipeline) > 0) {
    bots.leader = 'needs_you'
    view.phase = 'questions'
    return view
  }
  if (pipeline.current === 'plan' || kindOf(pipeline.stages.find((s) => s.id === pipeline.current) ?? ({ id: 'plan' } as FocusStage)) === 'plan') {
    view.phase = 'planning'
  }
  return view
}


// The plan stage's decision cards: the leader's questions before the plan.
function questionCards(pipeline: FocusPipeline): FocusCard[] {
  const planStages = new Set(pipeline.stages.filter((s) => kindOf(s) === 'plan').map((s) => s.id))
  return pipeline.cards.filter((c) => c.kind === 'decision' && planStages.has(c.stage))
}

function openQuestions(pipeline: FocusPipeline): number {
  if (pipeline.finished_at || pipeline.open_gate === 'plan') return 0
  return questionCards(pipeline).filter((c) => c.state !== 'decided').length
}

function questionsOf(pipeline: FocusPipeline): EasyQuestion[] {
  return questionCards(pipeline).map((card) => {
    const d = (card.payload ?? {}) as Partial<FocusDecision>
    const q: EasyQuestion = {
      cardId: card.id,
      question: d.question || card.title,
      options: Array.isArray(d.options) ? d.options.filter((o) => typeof o === 'string' && o.trim() !== '') : [],
      youDecide: Boolean(d.you_decide),
    }
    if (card.state === 'decided' && card.decision) q.answer = card.decision
    return q
  })
}

function isPartKind(kind: string): kind is EasyScreenPartKind {
  return (EASY_SCREEN_PARTS as readonly string[]).includes(kind)
}

function blueprintOf(plan: FocusPlan): EasyBlueprint {
  const words = (list: string[] | undefined) => (list ?? []).filter((w) => typeof w === 'string' && w.trim() !== '')
  return {
    goal: plan.goal,
    tasks: (plan.tasks ?? []).map((t) => t.title),
    screens: (plan.screens ?? []).map((s) => ({
      name: s.name,
      line: s.line ?? '',
      parts: (s.parts ?? []).filter((p) => isPartKind(p.kind)).map((p) => ({ kind: p.kind as EasyScreenPartKind, text: p.text ?? '' })),
    })),
    checks: (plan.checks ?? []).map((c) => c.label),
    saved: words(plan.saved),
    notNow: words(plan.not_now),
  }
}

// The checks with their latest results: the server's check_results, or,
// before it recorded any, the plan's checks as not run yet.
function checksOf(pipeline: FocusPipeline): EasyCheckRow[] {
  if (pipeline.check_results?.length) {
    return pipeline.check_results.map((r) => {
      const row: EasyCheckRow = { label: r.label, state: r.state }
      if (r.timed_out) row.timedOut = true
      if (r.state === 'failed' && r.check?.expected && r.check.seen) row.report = { screen: r.check.screen, expected: r.check.expected, seen: r.check.seen }
      return row
    })
  }
  return (pipeline.plan?.checks ?? []).map((c) => ({ label: c.label, state: 'not_run' as const }))
}

// How many checks passed, of how many: what the check screen counts.
export function checksPassed(checks: EasyCheckRow[]): { passed: number; total: number } {
  return { passed: checks.filter((c) => c.state === 'passed').length, total: checks.length }
}

function stageOf(pipeline: FocusPipeline): FocusStage | undefined {
  return pipeline.stages.find((s) => s.id === pipeline.current)
}

// The step a project is in: the plan until the blueprint is approved, then
// making; checking while the reviewer works, the checks run or a check has
// a result; done with all three once it is finished.
function stepOf(view: PhaseView, pipeline: FocusPipeline | null | undefined, checks: EasyCheckRow[]): EasyStep | null {
  if (!pipeline || view.phase === 'creating' || view.phase === 'failed') return 'plan'
  if (view.phase === 'finished') return null
  const stage = stageOf(pipeline)
  const kind = stage ? kindOf(stage) : 'plan'
  if (kind === 'plan') return 'plan'
  if (kind === 'review' || pipeline.awaiting_verification || checks.some((c) => c.state !== 'not_run')) return 'check'
  return 'make'
}

function stepsOf(step: EasyStep | null): Record<EasyStep, EasyStepState> {
  const steps = { plan: 'done', make: 'done', check: 'done', publish: 'next' } as Record<EasyStep, EasyStepState>
  if (!step) return steps
  const at = EASY_STEPS.indexOf(step)
  EASY_STEPS.forEach((s, i) => (steps[s] = i < at ? 'done' : i === at ? 'now' : 'next'))
  return steps
}

// The bot at work now, while the team works by itself.
function workingBot(view: PhaseView, pipeline: FocusPipeline | null | undefined): EasyBot | undefined {
  if (view.phase === 'creating') return 'leader'
  if (!pipeline || (view.phase !== 'building' && view.phase !== 'planning')) return undefined
  if (pipeline.awaiting_verification) return 'checker'
  const stage = stageOf(pipeline)
  if (!stage) return undefined
  const kind = kindOf(stage)
  if (kind === 'review') return pipeline.review?.fixing ? 'maker' : 'reviewer'
  return botOfKind[kind]
}

function logOf(project: EasyProject, pipeline: FocusPipeline | null | undefined, view: PhaseView, questions: EasyQuestion[], checks: EasyCheckRow[]): EasyLogEntry[] {
  const log: EasyLogEntry[] = view.change ? [{ who: 'you', line: 'change', text: view.change }] : [{ who: 'you', line: 'idea', text: project.idea }]
  if (!pipeline || view.phase === 'creating' || view.phase === 'failed') return log
  if (questions.length) log.push({ who: 'leader', line: 'asked' })
  const byKind = (kind: string) => pipeline.stages.find((s) => kindOf(s) === kind)
  const plan = byKind('plan')
  if (plan?.status === 'active' || !plan) {
    if (view.phase === 'approve') log.push({ who: 'leader', line: 'drew' })
    else if (view.phase === 'planning') log.push({ who: 'leader', line: 'drawing' })
    return log
  }
  if (pipeline.plan) log.push({ who: 'leader', line: 'handed' })
  const make = byKind('build')
  if (make?.status === 'active') log.push({ who: 'maker', line: 'making' })
  else if (make && make.status !== 'pending') log.push({ who: 'maker', line: 'made' })
  const review = byKind('review')
  if (review?.status === 'active') log.push({ who: pipeline.review?.fixing ? 'maker' : 'reviewer', line: pipeline.review?.fixing ? 'fixing' : 'reviewing' })
  const ran = checks.filter((c) => c.state !== 'not_run')
  if (ran.length) {
    const { passed, total } = checksPassed(checks)
    log.push({ who: 'checker', line: 'checked', passed, total })
    if (passed < total && checks.some((c) => c.state === 'failed') && view.phase === 'building') log.push({ who: 'maker', line: 'fixingCheck' })
  }
  if (view.phase === 'finished') log.push({ who: 'leader', line: 'finished' })
  return log
}

function workspace(project: EasyProject, pipeline: (FocusPipeline & SavedField) | null | undefined, view: PhaseView) {
  const questions = pipeline ? questionsOf(pipeline) : []
  const checks = pipeline ? checksOf(pipeline) : []
  const step = stepOf(view, pipeline, checks)
  const out: Pick<EasyView, 'step' | 'steps' | 'working' | 'questions' | 'blueprint' | 'checks' | 'log'> = {
    step: step ?? 'check',
    steps: stepsOf(step),
    questions,
    checks,
    log: logOf(project, pipeline, view, questions, checks),
  }
  const working = workingBot(view, pipeline)
  if (working) out.working = working
  if (pipeline?.plan) out.blueprint = blueprintOf(pipeline.plan)
  return out
}

// Whether the blueprint has anything beyond its tasks.
export function hasBlueprint(b: EasyBlueprint): boolean {
  return b.screens.length > 0 || b.checks.length > 0 || b.saved.length > 0 || b.notNow.length > 0
}

// Whether a project still changes on its own, so the screen keeps reading.
export function easyLive(view: EasyView): boolean {
  if (view.phase === 'finished') return view.saved === undefined
  return view.phase === 'creating' || view.phase === 'planning' || view.phase === 'building'
}


// Whether the project takes a change now: finished and saved (or its save
// failed), so the folder is free. The server checks the same.
export function easyCanChange(view: EasyView): boolean {
  return view.phase === 'finished' && view.saved !== undefined
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

// A saved version of a project (internal/easyproject/versions.go): the
// start the scaffold left, the work of a finished run, or a going back
// (whose title names the version it brought back).
export type EasyVersionKind = 'start' | 'saved' | 'restored'
export type EasyVersion = { commit: string; title: string; kind: EasyVersionKind; at: string; current?: boolean }

// How a version is named on the screen. The titles are the team's own
// plain words (the plan's goal); the start has no words of its own.
export function versionLabel(v: EasyVersion, words: { start: string; backTo: (title: string) => string }): string {
  switch (v.kind) {
    case 'start':
      return words.start
    case 'restored':
      return words.backTo(v.title)
    default:
      return v.title
  }
}

// The Start screen's answer (internal/easyproject/readiness.go): whether
// this computer has what a project needs, and the one next thing to do.
export type EasyCheckState = 'ok' | 'missing' | 'signed_out' | 'not_set'
export type EasyNextStep = 'choose_engine' | 'install_engine' | 'sign_in' | 'install_tools'
export type EasyReadiness = {
  ready: boolean
  next?: EasyNextStep
  engine: EasyCheckState
  engine_kind?: string
  tools: EasyCheckState
}

// How often the Start screen reads the answer again while the person
// installs or signs in.
export const startPollMs = 5000

// The commands the Start screen asks the person to type in Terminal: Claude
// Code's own installer and its first run, which signs in. Not words to
// translate.
export const START_COMMANDS = {
  installClaude: 'curl -fsSL https://claude.ai/install.sh | bash',
  signIn: 'claude',
} as const

// The engine's name as the person knows it.
export function engineName(kind: string | undefined): string {
  switch (kind ?? '') {
    case 'claude-code-cli':
    case '':
      return 'Claude Code'
    case 'antigravity-cli':
      return 'Antigravity'
    default:
      return 'AI'
  }
}

// Easy mode's first run (POST /v1/admin/setup/easy): with no AI set up yet,
// the server writes Claude Code on every tier and restarts, and the screen
// waits for it to come back set up. setUp resolves when the server took
// the setup or was already set up; health is GET /v1/healthz, which throws
// while the server restarts.
export type EasySetupAPI = {
  setUp: () => Promise<void>
  health: () => Promise<{ needs_setup?: boolean }>
  sleep: (ms: number) => Promise<void>
}

export type EasySetupResult = 'ready' | 'failed' | 'timeout'

export const easySetupPollMs = 1000
export const easySetupAttempts = 90

export async function runEasySetup(api: EasySetupAPI, attempts = easySetupAttempts): Promise<EasySetupResult> {
  try {
    await api.setUp()
  } catch {
    return 'failed'
  }
  for (let i = 0; i < attempts; i++) {
    await api.sleep(easySetupPollMs)
    try {
      const health = await api.health()
      if (!health.needs_setup) return 'ready'
    } catch {
      // Restarting: the connection is refused until it is back.
    }
  }
  return 'timeout'
}
