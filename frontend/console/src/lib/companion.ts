import type { ActivityMap } from './sessionBoard'
import type { NotificationMessage } from './types'

export type CompanionVisibilityInput = {
  enabled?: boolean
  needsSetup?: boolean
  loginRequired?: boolean
  zenActive?: boolean
}

export type CompanionLocale = 'en' | 'ko'

// CASE's face (#1190). Shape — eyes, mouth, eyebrows, antenna — carries the
// expression; glow colour is a hint, never the only signal (DESIGN.md
// Motion). `companionExpression` below is the one place that turns state
// into one of these.
export type CompanionExpression =
  | 'neutral'
  | 'working'
  | 'alert'
  | 'happy'
  | 'upset'
  | 'wary'
  | 'sleepy'
  | 'greeting'

export const COMPANION_EXPRESSIONS: readonly CompanionExpression[] = [
  'neutral',
  'working',
  'alert',
  'happy',
  'upset',
  'wary',
  'sleepy',
  'greeting',
]

// What CASE's bubble shows: one line per thing waiting on the user. `path`
// is where clicking the line goes, via lib/router.ts.
export type CompanionLineKind = 'pending' | 'queued' | 'running' | 'failure'

export type CompanionLine = {
  key: string
  kind: CompanionLineKind
  label: string
  path: string
}

// A recent failure noticed from the SSE stream (cron, pulse, watchdog,
// ops error/critical events). Kept for 30 minutes, newest 5 at most.
export type CompanionFailure = {
  key: string
  sessionId?: string
  label: string
  at: number
}

export type CompanionSnapshot = {
  // Count badge shown on the bot: approvals waiting (chat + unattended) +
  // recent failures. Running turns are not counted — they are not waiting
  // on the user.
  badge: number
  lines: CompanionLine[]
}

// Optional inputs for the expressions that are not driven by `lines`
// (#1190: the expression type and its picture exist now; the timers and
// events that fill these cues in are P3/#1191 scope). `companionExpression`
// only reads them when nothing in `lines` already says more — a real wait
// or failure always outranks a cue.
export type CompanionExpressionCues = {
  // A warning-severity event just arrived (not yet a failure).
  warning?: boolean
  // A turn just finished successfully.
  justFinished?: boolean
  // The user (or a notification) just arrived after being away.
  justArrived?: boolean
  // The console has been quiet for a while with nothing waiting.
  longQuiet?: boolean
}

export type CompanionLineText = {
  pending: (count: number, title: string) => string
  queued: (count: number, title: string) => string
  running: (title: string) => string
  failure: (label: string) => string
}

const failureWindowMs = 30 * 60 * 1000
const maxFailures = 5

export function shouldShowCompanion(input: CompanionVisibilityInput): boolean {
  return !!input.enabled && !input.needsSetup && !input.loginRequired && !input.zenActive
}

export function companionEnabledFromConfigValues(values?: Record<string, unknown>): boolean {
  if (!values) return false
  return values.companion_enabled === true
}

export function normalizeCompanionLocale(locale?: string | null): CompanionLocale {
  return (locale || '').trim().toLowerCase().startsWith('ko') ? 'ko' : 'en'
}

// companionState turns a chat-activity snapshot and recent failures into
// CASE's badge count and bubble lines. Pure: no fetch, no LLM call. Order:
// approvals waiting (pending, then queued), then recent failures, then
// running turns. A failure is listed ahead of a running turn on purpose —
// of the two, it is the more urgent signal.
//
// `activeSessionId` is the session the chat route currently has on screen
// (undefined/null when nothing is, e.g. the board). Its pending approval
// and running turn are left out of lines and the badge: that session's own
// approval card is already visible in its thread, and its running turn is
// already visible as the streaming reply, so CASE would otherwise
// duplicate what the user is already looking at (and, worse, cover the
// very panel they are using — #1194). Queued (unattended) approvals have
// no card in any thread, so they still show for the active session too;
// switching to another session brings its own lines back.
export function companionState(
  activity: ActivityMap,
  failures: CompanionFailure[],
  now: number,
  text: CompanionLineText,
  activeSessionId?: string | null,
): CompanionSnapshot {
  const entries = Object.entries(activity).sort(([a], [b]) => a.localeCompare(b))
  const visibleEntries = entries.filter(([sessionId]) => sessionId !== activeSessionId)
  const lines: CompanionLine[] = []

  for (const [sessionId, state] of visibleEntries) {
    if (state.pending > 0) {
      lines.push({
        key: `pending:${sessionId}`,
        kind: 'pending',
        label: text.pending(state.pending, state.title),
        path: `/console/chat/${sessionId}`,
      })
    }
  }
  for (const [sessionId, state] of entries) {
    if (state.queued > 0) {
      lines.push({
        key: `queued:${sessionId}`,
        kind: 'queued',
        label: text.queued(state.queued, state.title),
        path: '/console/ops',
      })
    }
  }
  const activeFailures = pruneFailures(failures, now)
  for (const failure of activeFailures) {
    lines.push({
      key: failure.key,
      kind: 'failure',
      label: text.failure(failure.label),
      path: failure.sessionId ? `/console/chat/${failure.sessionId}` : '/console/ops',
    })
  }
  for (const [sessionId, state] of visibleEntries) {
    if (state.running) {
      lines.push({
        key: `running:${sessionId}`,
        kind: 'running',
        label: text.running(state.title),
        path: `/console/chat/${sessionId}`,
      })
    }
  }

  const pendingTotal = visibleEntries.reduce((sum, [, s]) => sum + s.pending, 0)
  const queuedTotal = entries.reduce((sum, [, s]) => sum + s.queued, 0)
  const badge = pendingTotal + queuedTotal + activeFailures.length

  return { badge, lines }
}

// companionExpression turns CASE's snapshot (and, once P3/#1191 wires them
// up, optional cues about recent events) into one of the eight expressions
// (#1190). What the snapshot's `lines` already say always outranks a cue:
// a real failure or wait is never overridden by "just said hello". Order
// within `lines`: failure > pending/queued approval > running turn — the
// same urgency order `companionState` lists them in. Order within `cues`:
// warning > justFinished > justArrived > longQuiet.
export function companionExpression(
  snapshot: Pick<CompanionSnapshot, 'lines'>,
  cues?: CompanionExpressionCues,
): CompanionExpression {
  if (snapshot.lines.some((line) => line.kind === 'failure')) return 'upset'
  if (snapshot.lines.some((line) => line.kind === 'pending' || line.kind === 'queued')) return 'alert'
  if (snapshot.lines.some((line) => line.kind === 'running')) return 'working'
  if (cues?.warning) return 'wary'
  if (cues?.justFinished) return 'happy'
  if (cues?.justArrived) return 'greeting'
  if (cues?.longQuiet) return 'sleepy'
  return 'neutral'
}

// pruneFailures drops anything older than 30 minutes, de-duplicates by
// key — a replayed SSE event (a reconnect, or /v1/events/history replay)
// must not render as two bubble lines for the same failure, which would
// also break Svelte's keyed each — and keeps only the newest 5, so the
// bubble never grows into a log. A duplicate key keeps its latest
// occurrence's content and moves to that occurrence's position, so the
// newest-5 cut still means newest.
export function pruneFailures(failures: CompanionFailure[], now: number): CompanionFailure[] {
  const fresh = failures.filter((failure) => now - failure.at <= failureWindowMs)
  const byKey = new Map<string, CompanionFailure>()
  for (const failure of fresh) {
    byKey.delete(failure.key)
    byKey.set(failure.key, failure)
  }
  return Array.from(byKey.values()).slice(-maxFailures)
}

// companionFailureFromEvent turns a qualifying SSE notification (severity
// error/critical) into a failure line. Everything else (info, warn,
// success, embodiment) is not a failure and returns null.
export function companionFailureFromEvent(event: NotificationMessage, now: number): CompanionFailure | null {
  if (!event || event.type === 'keepalive') return null
  const severity = (event.severity || '').trim().toLowerCase()
  if (severity !== 'error' && severity !== 'critical') return null
  const title = clipText(event.title || event.category || 'failure', 70)
  const sessionId = event.session_id ? event.session_id.trim() : ''
  return {
    key: `failure:${event.id ?? `${event.category}:${event.timestamp}:${title}`}`,
    sessionId: sessionId || undefined,
    label: title,
    at: now,
  }
}

// companionWaitingKeys is the set of line keys that count as "something is
// waiting" (approvals and failures, not running turns) — the input to
// companionShouldOpen on the next tick.
export function companionWaitingKeys(lines: CompanionLine[]): Set<string> {
  return new Set(lines.filter((line) => line.kind !== 'running').map((line) => line.key))
}

// companionShouldOpen is true only when a line not present before is a new
// approval wait or a new failure: running turns and turns finishing never
// reopen the bubble by themselves. `primed` must be false for the very
// first evaluation after the component mounts, so whatever was already
// waiting when CASE first renders (e.g. the page was just reloaded) is
// treated as already-there, not new, and does not pop the bubble open.
export function companionShouldOpen(primed: boolean, prevWaitingKeys: ReadonlySet<string>, lines: CompanionLine[]): boolean {
  if (!primed) return false
  return lines.some((line) => line.kind !== 'running' && !prevWaitingKeys.has(line.key))
}

// companionShouldAutoClose is true once a bubble that opened itself has
// nothing left to show it for (its last approval wait or failure is gone —
// answered, navigated to, dismissed, or expired). A bubble the user opened
// by hand (`autoOpened: false`) is never closed by this: only the thing
// that opened it on its own closes it on its own.
export function companionShouldAutoClose(autoOpened: boolean, lines: CompanionLine[]): boolean {
  return autoOpened && companionWaitingKeys(lines).size === 0
}

export interface CompanionHandoff {
  // The chat message: the user's own words, so the bubble and the session
  // title show what they typed.
  prompt: string
  // Guidance for the model only, sent as `console_context` (see
  // lib/consoleContext.ts): the companion tone, the console area, and no
  // tools unless asked.
  context: string
}

export function companionHandoffForAsk(raw: string, routeView?: string, locale?: string | null): CompanionHandoff {
  const lang = normalizeCompanionLocale(locale)
  const prompt = clipText(raw.trim(), 600)
  const route = (routeView || 'unknown').trim()
  const context = lang === 'ko'
    ? [
        '이 메시지는 콘솔의 컴패니언에게 건넨 말이야. TARS 콘솔 안의 컴패니언처럼 답해줘.',
        `현재 콘솔 영역: ${companionAreaLabel(routeView, lang)} (${route}).`,
        '짧게 답하고, 실용적인 다음 행동 하나만 제안해. 내가 명시적으로 요청하지 않으면 도구를 실행하지 마.',
      ]
    : [
        'This message was said to the companion in the Console. Act as the TARS companion inside the Console.',
        `Current console area: ${companionAreaLabel(routeView, lang)} (${route}).`,
        'Answer briefly, give one practical next action, and do not run tools unless I explicitly ask.',
      ]
  return { prompt, context: context.join('\n') }
}

function companionAreaLabel(routeView?: string, locale: CompanionLocale = 'en'): string {
  const labels: Record<CompanionLocale, Record<string, string>> = {
    en: {
      chat: 'this chat',
      pulse: 'Pulse',
      reflection: 'Reflection',
      agentruntime: 'Agent Runtime',
      tasks: 'Plans',
      config: 'Settings',
      logs: 'Logs',
      analytics: 'Analytics',
      memory: 'Memory',
      home: 'Mission Control',
      default: 'the Console',
    },
    ko: {
      chat: '이 채팅',
      pulse: '펄스',
      reflection: '리플렉션',
      agentruntime: '에이전트 런타임',
      tasks: '계획',
      config: '설정',
      logs: '로그',
      analytics: '분석',
      memory: '메모리',
      home: '미션 컨트롤',
      default: '콘솔',
    },
  }
  return labels[locale][routeView || 'default'] || labels[locale].default
}

function clipText(value: string, max: number): string {
  const text = value.replace(/\s+/g, ' ').trim()
  if (text.length <= max) return text
  return `${text.slice(0, Math.max(0, max - 3)).trim()}...`
}
