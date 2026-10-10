import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  COMPANION_EXPRESSIONS,
  COMPANION_CUE_DURATIONS_MS,
  COMPANION_CUE_NONE,
  COMPANION_GAZE_EXCLUDED_EXPRESSIONS,
  COMPANION_INPUT_THROTTLE_MS,
  COMPANION_JUST_ARRIVED_HIDDEN_MS,
  COMPANION_LONG_QUIET_MS,
  COMPANION_SERVER_CUE_LINE_MS,
  COMPANION_SERVER_CUE_NO_LINE_MS,
  COMPANION_SERVER_EVENT_MAX_AGE_MS,
  companionActionFor,
  companionCueDismissServer,
  companionCueTick,
  companionCuesFor,
  companionEnabledFromConfigValues,
  companionFailedSessionIds,
  companionExpression,
  companionFailureFromEvent,
  companionGazeApplies,
  companionGazeOffset,
  companionHandoffForAsk,
  companionIdleEligible,
  companionIsLongQuiet,
  companionJustArrived,
  companionNextIdle,
  companionServerCueFromEvent,
  companionShouldAutoClose,
  companionShouldOpen,
  companionShouldRecordInput,
  companionState,
  companionTurnsJustFinished,
  companionWaitingKeys,
  companionWarningFromEvent,
  pruneFailures,
  shouldShowCompanion,
  type CompanionCueState,
  type CompanionExpressionCues,
  type CompanionFailure,
  type CompanionLine,
  type CompanionServerCue,
} from '../src/lib/companion.ts'
import { companionEn } from '../src/i18n/sections/companion.ts'
import type { ActivityMap } from '../src/lib/sessionBoard.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const componentSource = readFileSync(new URL('../src/components/CompanionPet.svelte', import.meta.url), 'utf8')
const helperSource = readFileSync(new URL('../src/lib/companion.ts', import.meta.url), 'utf8')

const now = Date.parse('2026-05-19T00:00:00Z')
const text = companionEn.lines

test('companion visibility is bounded by config and setup/auth state', () => {
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: false, zenActive: false }), true)
  assert.equal(shouldShowCompanion({ enabled: false, needsSetup: false, loginRequired: false, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: true, loginRequired: false, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: true, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: false, zenActive: true }), false)
})

test('console app wires the floating companion to live chat activity', () => {
  assert.match(appSource, /CompanionPet/)
  assert.match(appSource, /getConfigSchema/)
  assert.match(helperSource, /companion_enabled/)
  assert.match(appSource, /shouldShowCompanion/)
  assert.match(appSource, /companionFailureFromEvent/)
  assert.match(appSource, /activity=\{sessionActivity\.activity\}/)
  assert.match(appSource, /failures=\{companionFailures\}/)
  assert.match(appSource, /activeSessionId=\{activeChatSessionId\}/)
  assert.match(appSource, /onNavigate=\{navigate\}/)
  assert.match(appSource, /onDismissFailure=\{handleDismissCompanionFailure\}/)
  assert.match(appSource, /onAsk=\{handleCompanionAsk\}/)
  assert.match(appSource, /routeView=\{route\.view\}/)
  // The warn-cue signal (#1191): App turns a qualifying SSE event into a
  // timestamp and passes it down.
  assert.match(appSource, /companionWarningFromEvent/)
  assert.match(appSource, /warningAt=\{companionWarningAt\}/)
})

test('CASE pauses all its motion timers and listeners when the tab is hidden, and resumes them when it is visible again (#1191)', () => {
  assert.match(componentSource, /document\.addEventListener\('visibilitychange'/)
  assert.match(componentSource, /document\.removeEventListener\('visibilitychange'/)
  assert.match(componentSource, /document\.hidden/)
  // The idle schedule resumes from where it left off (an absolute fire
  // time), not a freshly rolled delay — pausing on a hidden tab must not
  // reset how soon CASE plays next.
  assert.match(componentSource, /idleFireAt/)
})

test('CASE turns off actions, idle play and cursor gaze under prefers-reduced-motion, without starting their timers at all', () => {
  assert.match(componentSource, /matchMedia\('\(prefers-reduced-motion: reduce\)'\)/)
  assert.match(componentSource, /prefersReducedMotion/)
})

test('CASE batches pointermove into one requestAnimationFrame via a passive listener, for the cursor gaze', () => {
  assert.match(componentSource, /addEventListener\('pointermove', onPointerMove, \{ passive: true \}\)/)
  assert.match(componentSource, /requestAnimationFrame\(/)
  assert.match(componentSource, /cancelAnimationFrame\(/)
})

test('"input" for longQuiet/sleepy also counts a drifting pointer and a scroll, throttled and passive, independent of reduced motion and the gaze listener', () => {
  // A dedicated listener set (not startGaze/onPointerMove, which is off
  // under reduced motion) covers pointerdown/keydown/pointermove/wheel.
  assert.match(componentSource, /function startInputTracking\(\)/)
  assert.match(componentSource, /addEventListener\('pointerdown', handleInput, \{ passive: true \}\)/)
  assert.match(componentSource, /addEventListener\('keydown', handleInput\)/)
  assert.match(componentSource, /addEventListener\('pointermove', handleInput, \{ passive: true \}\)/)
  assert.match(componentSource, /addEventListener\('wheel', handleInput, \{ passive: true \}\)/)
  // The write to `lastInputAt` is throttled through the pure
  // companionShouldRecordInput, not done unconditionally on every event.
  assert.match(componentSource, /companionShouldRecordInput\(lastInputAt, eventNow\)/)

  // Started unconditionally from onMount — no reduced-motion guard around
  // it, unlike startDecorativeMotion/startGaze.
  const mountStart = componentSource.indexOf('onMount(() => {')
  const mountBody = componentSource.slice(mountStart, componentSource.indexOf('return () => {', mountStart))
  assert.match(mountBody, /startInputTracking\(\)/)
  assert.doesNotMatch(mountBody, /if \(prefersReducedMotion\)/)

  // Pauses with the tab and cleans up on destroy, same as every other
  // listener here.
  assert.match(componentSource, /function stopInputTracking\(\)/)
  assert.match(componentSource, /stopInputTracking\(\)/)
  assert.match(componentSource, /removeEventListener\('pointermove', handleInput\)/)
  assert.match(componentSource, /removeEventListener\('wheel', handleInput\)/)
})

test('idle play never outlives a real expression change: cleared in JS, and its CSS only ever matches alongside expr-neutral', () => {
  // The component clears `idlePlay` the instant `expression` leaves
  // neutral, rather than waiting for the look/yawn/wink animation's own
  // `animationend` — which would not come anyway once the CSS below stops
  // matching (that cancels the animation instead of finishing it).
  assert.match(componentSource, /expression !== 'neutral' && idlePlay\) idlePlay = null/)

  // Every play-* selector requires .expr-neutral on the same element as
  // .companion-body, not just the play-* class: a stale `idlePlay` left
  // over from before an unrelated code path could set it (or a future
  // change that forgets the effect above) must still never apply a
  // look/yawn/wink animation on top of a real, non-neutral expression's
  // own eye/mouth shape.
  const style = componentSource.slice(componentSource.indexOf('<style>'))
  const rules = companionLeafRules(style)
  const playSelectors = rules
    .flatMap((rule) => rule.selectors)
    .filter((sel) => /\.play-(look|yawn|wink)\b/.test(sel))
  assert.ok(playSelectors.length > 0, 'expected at least one play-* selector to check')
  for (const sel of playSelectors) {
    assert.match(sel, /\.expr-neutral/, `${sel} does not require .expr-neutral`)
  }
})

test('CASE cleans up every timer and listener it starts when it is destroyed', () => {
  assert.match(componentSource, /onMount\(/)
  assert.match(componentSource, /return \(\) => \{/)
  assert.match(componentSource, /clearInterval\(cueTimer\)/)
  assert.match(componentSource, /clearTimeout\(idleTimer\)/)
  assert.match(componentSource, /removeEventListener\('pointerdown', handleInput\)/)
  assert.match(componentSource, /removeEventListener\('keydown', handleInput\)/)
})

test('the three fixed-script buttons and their reactions are gone', () => {
  for (const removed of [
    'companionReactionForStimulus',
    'companionStimulusReactions',
    'companionAskHandoffReaction',
    'companionReactionFromEvent',
    'CompanionStimulus',
    'CompanionUiText',
    'companionText',
    'companionUiText',
    'manualPriority',
    'activeAction',
    'companion-feedback-strip',
  ]) {
    assert.doesNotMatch(helperSource, new RegExp(removed), `${removed} still in lib/companion.ts`)
    assert.doesNotMatch(componentSource, new RegExp(removed), `${removed} still in CompanionPet.svelte`)
    assert.doesNotMatch(appSource, new RegExp(removed), `${removed} still in App.svelte`)
  }
})

test('companion pet is a floating console presence with a waiting-list bubble', () => {
  assert.match(componentSource, /aria-label=\{\$t\.companion\.buttonAria\}/)
  assert.match(componentSource, /companion-bubble/)
  assert.match(componentSource, /companion-lines/)
  assert.match(componentSource, /companion-badge/)
  assert.match(componentSource, /\$t\.companion\.header/)
  assert.match(componentSource, /\$t\.companion\.emptyLine/)
  assert.match(componentSource, /position:\s*fixed;/)
  assert.match(componentSource, /bottom:\s*var\(--space-5\);/)
  assert.match(componentSource, /right:\s*var\(--space-5\);/)
  assert.match(componentSource, /pointer-events:\s*auto;/)
  assert.match(componentSource, /@keyframes companionBlink/)
})

test('a failure line can be dismissed without navigating, and navigating away dismisses it too', () => {
  // A dedicated × per failure line (dismiss without leaving the page)...
  assert.match(componentSource, /companion-line-dismiss/)
  assert.match(componentSource, /aria-label=\{\$t\.companion\.dismissFailureAria\}/)
  assert.match(componentSource, /onclick=\{\(\) => dismissFailure\(line\.key\)\}/)
  // ...and clicking the line itself (which navigates) dismisses a failure too.
  assert.match(componentSource, /if \(line\.kind === 'failure'\) onDismissFailure\?\.\(line\.key\)/)
})

// Review finding (#1192): a server line with a session_id is a button
// whose accessible name must be its own text (the server's message), same
// as every other .companion-line button here — an aria-label would hide
// that text from assistive tech instead of adding to it.
test('the session-linked server line has no aria-label overriding its own text', () => {
  const buttonBlockStart = componentSource.indexOf('class="companion-line companion-line-server"')
  assert.notEqual(buttonBlockStart, -1, 'expected the session-linked server line button in CompanionPet.svelte')
  const buttonBlockEnd = componentSource.indexOf('</button>', buttonBlockStart)
  const buttonBlock = componentSource.slice(buttonBlockStart, buttonBlockEnd)
  assert.doesNotMatch(buttonBlock, /aria-label/)
  assert.match(buttonBlock, /\{serverLine\.line\}/)
  assert.doesNotMatch(componentSource, /serverLineAria/)
  assert.doesNotMatch(appSource, /serverLineAria/)
})

test('the companion text addresses the user, not itself', () => {
  assert.equal(companionEn.emptyLine, 'Nothing is waiting on you right now.')
  assert.equal(companionEn.badgeAria(3), '3 waiting on you')
  assert.doesNotMatch(companionEn.emptyLine, /waiting on me/)
  assert.doesNotMatch(companionEn.badgeAria(1), /waiting on me/)
})

test('an empty activity snapshot is the neutral expression with no badge and no lines', () => {
  const snapshot = companionState({}, [], now, text)
  assert.deepEqual(snapshot, { badge: 0, lines: [] })
  assert.equal(companionExpression(snapshot), 'neutral')
})

test('a pending chat approval is an alert-expression line to that session', () => {
  const activity: ActivityMap = { s1: { running: false, pending: 2, queued: 0, title: 'Refactor session' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(companionExpression(snapshot), 'alert')
  assert.equal(snapshot.badge, 2)
  assert.deepEqual(snapshot.lines, [
    { key: 'pending:s1', kind: 'pending', label: text.pending(2, 'Refactor session'), path: '/console/chat/s1' },
  ])
})

test('a queued unattended approval is an alert-expression line to Ops', () => {
  const activity: ActivityMap = { s2: { running: false, pending: 0, queued: 1, title: 'Nightly cron' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(companionExpression(snapshot), 'alert')
  assert.equal(snapshot.badge, 1)
  assert.deepEqual(snapshot.lines, [
    { key: 'queued:s2', kind: 'queued', label: text.queued(1, 'Nightly cron'), path: '/console/ops' },
  ])
})

test('a running turn alone is the working expression with a line and no badge', () => {
  const activity: ActivityMap = { s3: { running: true, pending: 0, queued: 0, title: 'Long task' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(companionExpression(snapshot), 'working')
  assert.equal(snapshot.badge, 0)
  assert.deepEqual(snapshot.lines, [
    { key: 'running:s3', kind: 'running', label: text.running('Long task'), path: '/console/chat/s3' },
  ])
})

test('the session the chat route is currently showing is left out of pending and running — its own thread already shows them (#1194)', () => {
  const activity: ActivityMap = {
    active: { running: true, pending: 1, queued: 0, title: 'On screen' },
    other: { running: false, pending: 1, queued: 0, title: 'Elsewhere' },
  }
  const snapshot = companionState(activity, [], now, text, 'active')
  assert.equal(companionExpression(snapshot), 'alert')
  assert.equal(snapshot.badge, 1)
  assert.deepEqual(snapshot.lines, [
    { key: 'pending:other', kind: 'pending', label: text.pending(1, 'Elsewhere'), path: '/console/chat/other' },
  ])

  // Moving away from "active" (no activeSessionId filter, or a different
  // one) brings its lines back.
  const afterLeaving = companionState(activity, [], now, text, null)
  assert.deepEqual(afterLeaving.lines, [
    { key: 'pending:active', kind: 'pending', label: text.pending(1, 'On screen'), path: '/console/chat/active' },
    { key: 'pending:other', kind: 'pending', label: text.pending(1, 'Elsewhere'), path: '/console/chat/other' },
    { key: 'running:active', kind: 'running', label: text.running('On screen'), path: '/console/chat/active' },
  ])
})

test('a queued (unattended) approval still shows for the currently active session — it has no card in any thread', () => {
  const activity: ActivityMap = { active: { running: false, pending: 0, queued: 1, title: 'On screen' } }
  const snapshot = companionState(activity, [], now, text, 'active')
  assert.equal(snapshot.badge, 1)
  assert.deepEqual(snapshot.lines, [
    { key: 'queued:active', kind: 'queued', label: text.queued(1, 'On screen'), path: '/console/ops' },
  ])
})

test('a recent failure is the upset expression, outranking alert and working', () => {
  const failures: CompanionFailure[] = [{ key: 'failure:1', sessionId: 's4', label: 'Cron failed', at: now }]
  const activity: ActivityMap = { s4: { running: true, pending: 1, queued: 0, title: 'Mixed session' } }
  const snapshot = companionState(activity, failures, now, text)
  assert.equal(companionExpression(snapshot), 'upset')
  assert.equal(snapshot.badge, 2)
  assert.deepEqual(snapshot.lines, [
    { key: 'pending:s4', kind: 'pending', label: text.pending(1, 'Mixed session'), path: '/console/chat/s4' },
    { key: 'failure:1', kind: 'failure', label: text.failure('Cron failed'), path: '/console/chat/s4' },
    { key: 'running:s4', kind: 'running', label: text.running('Mixed session'), path: '/console/chat/s4' },
  ])
})

test('a sessionless failure points at Ops', () => {
  const failures: CompanionFailure[] = [{ key: 'failure:2', label: 'Watchdog failed', at: now }]
  const snapshot = companionState({}, failures, now, text)
  assert.equal(snapshot.lines[0].path, '/console/ops')
})

test('a failure for the currently active session still shows — unlike pending/running, it has no card in that thread', () => {
  const failures: CompanionFailure[] = [{ key: 'failure:3', sessionId: 'active', label: 'Cron failed', at: now }]
  const snapshot = companionState({}, failures, now, text, 'active')
  assert.deepEqual(snapshot.lines, [
    { key: 'failure:3', kind: 'failure', label: text.failure('Cron failed'), path: '/console/chat/active' },
  ])
})

test('companionExpression picks neutral with no lines and no cues', () => {
  assert.equal(companionExpression({ lines: [] }), 'neutral')
  assert.equal(companionExpression({ lines: [] }, {}), 'neutral')
})

test('companionExpression: a cue alone (no lines) picks its own expression, in server > warning > justFinished > justArrived > longQuiet order', () => {
  const noLines: CompanionLine[] = []
  assert.equal(companionExpression({ lines: noLines }, { server: 'sleepy' }), 'sleepy')
  assert.equal(companionExpression({ lines: noLines }, { warning: true }), 'wary')
  assert.equal(companionExpression({ lines: noLines }, { justFinished: true }), 'happy')
  assert.equal(companionExpression({ lines: noLines }, { justArrived: true }), 'greeting')
  assert.equal(companionExpression({ lines: noLines }, { longQuiet: true }), 'sleepy')

  // Higher-priority cues outrank lower ones when several fire at once —
  // a server-requested expression outranks every cosmetic cue (#1192).
  const all: CompanionExpressionCues = { server: 'happy', warning: true, justFinished: true, justArrived: true, longQuiet: true }
  assert.equal(companionExpression({ lines: noLines }, all), 'happy')
  assert.equal(
    companionExpression({ lines: noLines }, { warning: true, justFinished: true, justArrived: true, longQuiet: true }),
    'wary',
  )
  assert.equal(companionExpression({ lines: noLines }, { justFinished: true, justArrived: true, longQuiet: true }), 'happy')
  assert.equal(companionExpression({ lines: noLines }, { justArrived: true, longQuiet: true }), 'greeting')
})

test('companionExpression: real state in lines always outranks every cue, including a server-requested one', () => {
  const failureLine: CompanionLine = { key: 'failure:1', kind: 'failure', label: 'x', path: '/console/ops' }
  const pendingLine: CompanionLine = { key: 'pending:s1', kind: 'pending', label: 'x', path: '/console/chat/s1' }
  const queuedLine: CompanionLine = { key: 'queued:s1', kind: 'queued', label: 'x', path: '/console/ops' }
  const runningLine: CompanionLine = { key: 'running:s1', kind: 'running', label: 'x', path: '/console/chat/s1' }
  const allCues: CompanionExpressionCues = { server: 'greeting', warning: true, justFinished: true, justArrived: true, longQuiet: true }

  assert.equal(companionExpression({ lines: [failureLine, runningLine] }, allCues), 'upset')
  assert.equal(companionExpression({ lines: [pendingLine, runningLine] }, allCues), 'alert')
  assert.equal(companionExpression({ lines: [queuedLine] }, allCues), 'alert')
  assert.equal(companionExpression({ lines: [runningLine] }, allCues), 'working')
})

test('COMPANION_EXPRESSIONS lists exactly the eight expressions once each', () => {
  assert.equal(COMPANION_EXPRESSIONS.length, 8)
  assert.equal(new Set(COMPANION_EXPRESSIONS).size, 8)
  assert.deepEqual(
    [...COMPANION_EXPRESSIONS].sort(),
    ['alert', 'greeting', 'happy', 'neutral', 'sleepy', 'upset', 'wary', 'working'],
  )
})

test('every expression has its own face shape rule in the component styles', () => {
  for (const expression of COMPANION_EXPRESSIONS) {
    assert.match(componentSource, new RegExp(`\\.expr-${expression}\\b`), `no .expr-${expression} rule in CompanionPet.svelte`)
  }
})

test('companionFailedSessionIds collects session ids from recent failures only', () => {
  const failures: CompanionFailure[] = [
    { key: 'failure:1', sessionId: 's1', label: 'x', at: now },
    { key: 'failure:2', label: 'no session', at: now },
    { key: 'failure:old', sessionId: 's9', label: 'old', at: now - 31 * 60 * 1000 },
  ]
  const ids = companionFailedSessionIds(failures, now)
  assert.deepEqual([...ids], ['s1'])
})

test('companionTurnsJustFinished is true only for another session\'s running turn ending without a failure', () => {
  const prev: ActivityMap = {
    s1: { running: true, pending: 0, queued: 0, title: 't1' },
    s2: { running: true, pending: 0, queued: 0, title: 't2' },
  }
  // Both still running: no transition for either.
  const bothStillRunning: ActivityMap = {
    s1: { running: true, pending: 0, queued: 0, title: 't1' },
    s2: { running: true, pending: 0, queued: 0, title: 't2' },
  }
  assert.equal(companionTurnsJustFinished(prev, bothStillRunning, null, new Set()), false)

  // s1 stopped running (s2 still running) and is not in the failed set: finished.
  const oneDone: ActivityMap = { s2: { running: true, pending: 0, queued: 0, title: 't2' } }
  assert.equal(companionTurnsJustFinished(prev, oneDone, null, new Set()), true)

  // s1 stopped running but is in the failed set: that is upset, not happy.
  assert.equal(companionTurnsJustFinished(prev, oneDone, null, new Set(['s1'])), false)

  // The session the chat route has on screen ending is left out — its own
  // thread already shows the reply arriving. s2 is still running, so this
  // only suppresses s1's own transition, not a real one elsewhere.
  assert.equal(companionTurnsJustFinished(prev, oneDone, 's1', new Set()), false)

  // Nothing was running before: no transition to detect.
  assert.equal(companionTurnsJustFinished({}, {}, null, new Set()), false)
})

test('companionWarningFromEvent is true only for a warn-severity notification', () => {
  assert.equal(companionWarningFromEvent({ type: 'notification', category: 'pulse', severity: 'warn', title: 'x', message: '', timestamp: '' }), true)
  assert.equal(companionWarningFromEvent({ type: 'notification', category: 'pulse', severity: 'info', title: 'x', message: '', timestamp: '' }), false)
  assert.equal(companionWarningFromEvent({ type: 'notification', category: 'pulse', severity: 'error', title: 'x', message: '', timestamp: '' }), false)
  assert.equal(companionWarningFromEvent({ type: 'keepalive', category: '', severity: 'warn', title: '', message: '', timestamp: '' }), false)
  // A category "companion" event is never a warning line, no matter what
  // severity it happens to carry — #1192 routes it through
  // companionServerCueFromEvent instead.
  assert.equal(companionWarningFromEvent({ type: 'notification', category: 'companion', severity: 'warn', title: '', message: 'hi', timestamp: '' }), false)
})

test('companionJustArrived fires only once a tab has been hidden ten minutes or more', () => {
  assert.equal(companionJustArrived(COMPANION_JUST_ARRIVED_HIDDEN_MS - 1), false)
  assert.equal(companionJustArrived(COMPANION_JUST_ARRIVED_HIDDEN_MS), true)
  assert.equal(companionJustArrived(60_000), false)
})

test('companionIsLongQuiet is level state: quiet for five minutes with nothing waiting, clears the instant something is waiting', () => {
  assert.equal(companionIsLongQuiet(now, now - COMPANION_LONG_QUIET_MS, false), true)
  assert.equal(companionIsLongQuiet(now, now - (COMPANION_LONG_QUIET_MS - 1), false), false)
  // A waiting line always wins, no matter how long the input has been quiet.
  assert.equal(companionIsLongQuiet(now, now - COMPANION_LONG_QUIET_MS * 10, true), false)
})

test('companionShouldRecordInput throttles to at most once per second, but always passes after a long gap', () => {
  assert.equal(companionShouldRecordInput(now, now), false)
  assert.equal(companionShouldRecordInput(now, now + COMPANION_INPUT_THROTTLE_MS - 1), false)
  assert.equal(companionShouldRecordInput(now, now + COMPANION_INPUT_THROTTLE_MS), true)
  // Waking a sleeping CASE: the gap since the last recorded input is huge
  // (minutes, not under a second), so the very first pointermove/wheel/
  // click/keystroke after a long quiet stretch always passes, not just
  // every-other one once the throttle window happens to align.
  assert.equal(companionShouldRecordInput(now - COMPANION_LONG_QUIET_MS, now), true)
})

test('companionCueTick: a higher-priority signal replaces a lower one, a lower one never interrupts, and each cue expires on its own', () => {
  let state: CompanionCueState = COMPANION_CUE_NONE

  // justArrived starts the cue and sets its own expiry.
  state = companionCueTick(state, now, { justArrived: true })
  assert.deepEqual(state, { cue: 'justArrived', expiresAt: now + COMPANION_CUE_DURATIONS_MS.justArrived })

  // A lower-or-equal-priority signal does not interrupt it before it expires.
  state = companionCueTick(state, now + 1_000, { justArrived: true })
  assert.equal(state.cue, 'justArrived')

  // A higher-priority signal (justFinished) replaces it immediately.
  state = companionCueTick(state, now + 2_000, { justFinished: true })
  assert.deepEqual(state, { cue: 'justFinished', expiresAt: now + 2_000 + COMPANION_CUE_DURATIONS_MS.justFinished })

  // warning outranks justFinished the same way.
  state = companionCueTick(state, now + 2_500, { warning: true, justArrived: true })
  assert.equal(state.cue, 'warning')

  // Once its own duration has elapsed with nothing new firing, it clears.
  const expiresAt = state.expiresAt as number
  state = companionCueTick(state, expiresAt + 1, {})
  assert.deepEqual(state, COMPANION_CUE_NONE)

  // No signal on an already-empty state stays empty.
  assert.deepEqual(companionCueTick(COMPANION_CUE_NONE, now, {}), COMPANION_CUE_NONE)
})

test('companionCueTick: a server cue outranks every other cue, holds for the line/no-line duration, and a newer one replaces an older one', () => {
  // server outranks warning even though warning already shows.
  let state = companionCueTick(COMPANION_CUE_NONE, now, { warning: true })
  const bodyOnly: CompanionServerCue = { expression: 'greeting' }
  state = companionCueTick(state, now + 1_000, { server: bodyOnly })
  assert.deepEqual(state, { cue: 'server', expiresAt: now + 1_000 + COMPANION_SERVER_CUE_NO_LINE_MS, server: bodyOnly })

  // A lower-priority signal (warning) does not interrupt the still-active
  // server cue.
  state = companionCueTick(state, now + 1_500, { warning: true })
  assert.equal(state.cue, 'server')

  // A line gets the much longer duration, not the body_only one.
  const withLine: CompanionServerCue = { expression: 'alert', line: 'approve this', sessionId: 's1' }
  state = companionCueTick(state, now + 2_000, { server: withLine })
  assert.deepEqual(state, { cue: 'server', expiresAt: now + 2_000 + COMPANION_SERVER_CUE_LINE_MS, server: withLine })

  // Its own duration elapsing with nothing new firing clears it, same as
  // every other cue.
  const expiresAt = state.expiresAt as number
  state = companionCueTick(state, expiresAt + 1, {})
  assert.deepEqual(state, COMPANION_CUE_NONE)
})

// Review finding (#1192): a later body_only companion event must not
// silently erase a still-showing line before its 5-minute hold, a
// dismiss, or a click — only a *newer line* may replace one.
test('companionCueTick: a still-showing server line only yields to a newer line, never to a later body_only event', () => {
  const withLine: CompanionServerCue = { expression: 'alert', line: 'approve X', sessionId: 's1' }
  let state = companionCueTick(COMPANION_CUE_NONE, now, { server: withLine })
  assert.deepEqual(state, { cue: 'server', expiresAt: now + COMPANION_SERVER_CUE_LINE_MS, server: withLine })

  // A later body_only event (same 'server' priority) does not touch the
  // still-showing line or its expiry — it is ignored outright.
  const bodyOnly: CompanionServerCue = { expression: 'neutral' }
  state = companionCueTick(state, now + 3_000, { server: bodyOnly })
  assert.deepEqual(state, { cue: 'server', expiresAt: now + COMPANION_SERVER_CUE_LINE_MS, server: withLine })

  // A later event that itself has a line is a legitimate update and still
  // replaces — including replacing another still-showing line — with its
  // own fresh duration.
  const newerLine: CompanionServerCue = { expression: 'happy', line: 'all done', sessionId: 's2' }
  state = companionCueTick(state, now + 4_000, { server: newerLine })
  assert.deepEqual(state, { cue: 'server', expiresAt: now + 4_000 + COMPANION_SERVER_CUE_LINE_MS, server: newerLine })

  // Once the line expires on its own, a later body_only event starts a
  // fresh (no-line) cue as normal — the hold-the-line rule only applies
  // while a line is actually still showing.
  const afterExpiry = (now + 4_000 + COMPANION_SERVER_CUE_LINE_MS) + 1
  state = companionCueTick(state, afterExpiry, { server: bodyOnly })
  assert.deepEqual(state, { cue: 'server', expiresAt: afterExpiry + COMPANION_SERVER_CUE_NO_LINE_MS, server: bodyOnly })
})

test('companionCueDismissServer clears an active server cue but leaves any other cue alone', () => {
  const serverState: CompanionCueState = { cue: 'server', expiresAt: now + 1000, server: { expression: 'happy', line: 'done' } }
  assert.deepEqual(companionCueDismissServer(serverState), COMPANION_CUE_NONE)

  const warningState: CompanionCueState = { cue: 'warning', expiresAt: now + 1000 }
  assert.deepEqual(companionCueDismissServer(warningState), warningState)
  assert.deepEqual(companionCueDismissServer(COMPANION_CUE_NONE), COMPANION_CUE_NONE)
})

test('companionServerCueFromEvent validates a category "companion" event into a CompanionServerCue, or ignores the whole event', () => {
  const freshTimestamp = new Date(now).toISOString()
  const base = { type: 'notification', category: 'companion', severity: 'info', title: '', timestamp: freshTimestamp }

  // A valid expression with a line and session_id, nobody looking at that session.
  assert.deepEqual(
    companionServerCueFromEvent({ ...base, message: 'approval needed', expression: 'alert', session_id: 's1' }, now, 's2'),
    { expression: 'alert', line: 'approval needed', sessionId: 's1' },
  )

  // body_only: no message at all is expression-only, regardless of session_id.
  assert.deepEqual(
    companionServerCueFromEvent({ ...base, message: '', expression: 'greeting', session_id: 's1' }, now, null),
    { expression: 'greeting' },
  )

  // The session named is the one already on screen: its own thread already
  // shows the message, so only the expression carries over.
  assert.deepEqual(
    companionServerCueFromEvent({ ...base, message: 'already visible', expression: 'happy', session_id: 's1' }, now, 's1'),
    { expression: 'happy' },
  )

  // No session_id at all: always a line (when there is a message), never suppressed.
  assert.deepEqual(
    companionServerCueFromEvent({ ...base, message: 'no session here', expression: 'wary' }, now, 's1'),
    { expression: 'wary', line: 'no session here' },
  )

  // An expression not in COMPANION_EXPRESSIONS ignores the whole event.
  assert.equal(companionServerCueFromEvent({ ...base, message: 'x', expression: 'curious' }, now, null), null)
  assert.equal(companionServerCueFromEvent({ ...base, message: 'x', expression: '' }, now, null), null)

  // Not category "companion": not this function's event at all.
  assert.equal(companionServerCueFromEvent({ ...base, category: 'pulse', message: 'x', expression: 'alert' }, now, null), null)

  // A keepalive frame is never a companion event even if it somehow carried fields.
  assert.equal(companionServerCueFromEvent({ type: 'keepalive', category: 'companion', severity: '', title: '', message: 'x', timestamp: freshTimestamp, expression: 'alert' }, now, null), null)

  // A coalesced replay is ignored (companion events are never stored/coalesced
  // server-side, but this guards a redelivery regardless).
  assert.equal(companionServerCueFromEvent({ ...base, message: 'x', expression: 'alert', coalesced: true }, now, null), null)

  // A missing or unparseable timestamp ignores the event.
  assert.equal(companionServerCueFromEvent({ ...base, timestamp: '', message: 'x', expression: 'alert' }, now, null), null)
  assert.equal(companionServerCueFromEvent({ ...base, timestamp: 'not-a-date', message: 'x', expression: 'alert' }, now, null), null)

  // Older than COMPANION_SERVER_EVENT_MAX_AGE_MS (a reconnect/replay) is ignored.
  const staleTimestamp = new Date(now - COMPANION_SERVER_EVENT_MAX_AGE_MS - 1).toISOString()
  assert.equal(companionServerCueFromEvent({ ...base, timestamp: staleTimestamp, message: 'x', expression: 'alert' }, now, null), null)
  // Exactly at the boundary still counts.
  const boundaryTimestamp = new Date(now - COMPANION_SERVER_EVENT_MAX_AGE_MS).toISOString()
  assert.notEqual(companionServerCueFromEvent({ ...base, timestamp: boundaryTimestamp, message: 'x', expression: 'alert' }, now, null), null)

  // The line is plain text only, whitespace-normalized and clipped to 140
  // chars like every other label in this file (clipText) — never HTML.
  const long = `<b>bold</b> ${'a'.repeat(200)}`
  const clipped = companionServerCueFromEvent({ ...base, message: long, expression: 'neutral' }, now, null)
  assert.ok(clipped?.line && clipped.line.length <= 140)
  assert.equal(clipped?.line, long.slice(0, 137).trim() + '...')
})

test('companionCuesFor maps the single active cue (at most one of four) plus longQuiet', () => {
  assert.deepEqual(companionCuesFor(COMPANION_CUE_NONE, false), {
    server: undefined,
    warning: false,
    justFinished: false,
    justArrived: false,
    longQuiet: false,
  })
  assert.deepEqual(companionCuesFor({ cue: 'warning', expiresAt: now }, false), {
    server: undefined,
    warning: true,
    justFinished: false,
    justArrived: false,
    longQuiet: false,
  })
  // longQuiet is independent of the transient cue.
  assert.deepEqual(companionCuesFor({ cue: 'justFinished', expiresAt: now }, true), {
    server: undefined,
    warning: false,
    justFinished: true,
    justArrived: false,
    longQuiet: true,
  })
  // A 'server' cue surfaces its own expression (#1192), not a fixed one.
  const serverState: CompanionCueState = { cue: 'server', expiresAt: now, server: { expression: 'greeting', line: 'hi', sessionId: 's1' } }
  assert.deepEqual(companionCuesFor(serverState, false), {
    server: 'greeting',
    warning: false,
    justFinished: false,
    justArrived: false,
    longQuiet: false,
  })
})

test('companionActionFor plays once on a transition into an action expression, and not again while it holds', () => {
  assert.equal(companionActionFor(null, 'alert'), 'bounce')
  assert.equal(companionActionFor('neutral', 'alert'), 'bounce')
  assert.equal(companionActionFor('alert', 'alert'), null) // same expression: no replay
  assert.equal(companionActionFor('neutral', 'happy'), 'nod')
  assert.equal(companionActionFor('neutral', 'upset'), 'shake')
  assert.equal(companionActionFor('neutral', 'wary'), 'tilt')
  assert.equal(companionActionFor('neutral', 'greeting'), 'wave')
  // working and sleepy are continuous CSS states, not one-shot actions.
  assert.equal(companionActionFor('neutral', 'working'), null)
  assert.equal(companionActionFor('neutral', 'sleepy'), null)
  assert.equal(companionActionFor('alert', 'neutral'), null)
})

test('companionNextIdle picks a 60–180s delay and a look/yawn/wink kind from an injected random source, deterministically', () => {
  assert.deepEqual(companionNextIdle(() => 0), { delayMs: 60_000, kind: 'look' })
  assert.deepEqual(companionNextIdle(() => 0.999), { delayMs: 179_880, kind: 'wink' })
  const mid = companionNextIdle(() => 0.5)
  assert.equal(mid.delayMs, 120_000)
  assert.equal(mid.kind, 'yawn')
})

test('companionIdleEligible requires neutral, nothing waiting, and a closed bubble', () => {
  assert.equal(companionIdleEligible('neutral', 0, false), true)
  assert.equal(companionIdleEligible('working', 0, false), false)
  assert.equal(companionIdleEligible('neutral', 1, false), false)
  assert.equal(companionIdleEligible('neutral', 0, true), false)
})

test('companionGazeOffset is 0 past the radius and scales up to maxOffset at its edge, both axes clamped', () => {
  const center = { x: 100, y: 100 }
  assert.deepEqual(companionGazeOffset(center, { x: 100, y: 100 }), { x: 0, y: 0 })
  // Exactly at the pointer: no lean.
  assert.deepEqual(companionGazeOffset(center, { x: 500, y: 100 }), { x: 0, y: 0 }) // far outside 200px radius
  // Straight right, halfway to the radius: half the max offset.
  const half = companionGazeOffset(center, { x: 200, y: 100 })
  assert.equal(Math.round(half.x), 2) // 100px of 200px radius -> ~1.5px rounds to 2 at this geometry
  assert.equal(half.y, 0)
  // Straight right, at the radius edge: full max offset, still clamped.
  const edge = companionGazeOffset(center, { x: 300, y: 100 })
  assert.equal(edge.x, 3)
  // Diagonal pointer still keeps each axis within [-max, max].
  const diag = companionGazeOffset(center, { x: 50, y: 50 })
  assert.ok(diag.x >= -3 && diag.x <= 3)
  assert.ok(diag.y >= -3 && diag.y <= 3)
})

test('companionGazeApplies is false only for the four non-pupil expressions', () => {
  for (const expression of COMPANION_GAZE_EXCLUDED_EXPRESSIONS) {
    assert.equal(companionGazeApplies(expression), false)
  }
  assert.equal(companionGazeApplies('neutral'), true)
  assert.equal(companionGazeApplies('working'), true)
  assert.equal(companionGazeApplies('alert'), true)
  assert.equal(companionGazeApplies('wary'), true)
})

test('failures older than 30 minutes are dropped, and only the newest 5 are kept', () => {
  const old: CompanionFailure = { key: 'failure:old', label: 'Old failure', at: now - 31 * 60 * 1000 }
  const fresh: CompanionFailure[] = Array.from({ length: 7 }, (_, i) => ({
    key: `failure:${i}`,
    label: `Failure ${i}`,
    at: now - i * 1000,
  }))
  const pruned = pruneFailures([old, ...fresh], now)
  assert.equal(pruned.length, 5)
  assert.ok(!pruned.some((f) => f.key === 'failure:old'))
})

test('pruneFailures de-duplicates by key, keeping the latest occurrence and its position', () => {
  // A replayed SSE event (a reconnect, or an /v1/events/history replay)
  // must not render as two bubble lines for the same failure — that would
  // also break Svelte's keyed each over snapshot.lines.
  const first: CompanionFailure = { key: 'failure:1', label: 'First message', at: now - 1000 }
  const other: CompanionFailure = { key: 'failure:2', label: 'Other failure', at: now - 500 }
  const replay: CompanionFailure = { key: 'failure:1', label: 'Replayed message', at: now }
  const pruned = pruneFailures([first, other, replay], now)
  assert.equal(pruned.length, 2)
  // The replay's content wins, and it moves to the position of its own
  // (later) occurrence, so a newest-5 cut still means newest.
  assert.deepEqual(pruned, [other, replay])
  assert.equal(pruned.filter((f) => f.key === 'failure:1').length, 1)
})

test('companionFailureFromEvent only turns error/critical severities into failures', () => {
  assert.equal(companionFailureFromEvent({ type: 'notification', category: 'ops', severity: 'info', title: 'x', message: '', timestamp: '' }, now), null)
  assert.equal(companionFailureFromEvent({ type: 'notification', category: 'pulse', severity: 'warn', title: 'x', message: '', timestamp: '' }, now), null)
  assert.equal(companionFailureFromEvent({ type: 'keepalive', category: '', severity: '', title: '', message: '', timestamp: '' }, now), null)

  const failure = companionFailureFromEvent(
    { type: 'notification', category: 'cron', severity: 'error', title: 'Cron failed', message: 'nightly job failed', timestamp: '2026-05-19T00:00:00Z', session_id: 's9' },
    now,
  )
  assert.equal(failure?.label, 'Cron failed')
  assert.equal(failure?.sessionId, 's9')
  assert.equal(failure?.at, now)

  const critical = companionFailureFromEvent(
    { type: 'notification', category: 'watchdog', severity: 'critical', title: 'Watchdog failed', message: '', timestamp: '2026-05-19T00:00:00Z' },
    now,
  )
  assert.equal(critical?.sessionId, undefined)

  // A category "companion" event is never a failure line either, even with
  // severity error/critical (#1192 routes it through
  // companionServerCueFromEvent instead).
  assert.equal(
    companionFailureFromEvent(
      { type: 'notification', category: 'companion', severity: 'critical', title: '', message: 'uh oh', timestamp: '2026-05-19T00:00:00Z' },
      now,
    ),
    null,
  )
})

test('the bubble opens itself only for a new approval wait or a new failure', () => {
  const emptyKeys = new Set<string>()
  const pendingLines = companionState({ s1: { running: false, pending: 1, queued: 0, title: 't' } }, [], now, text).lines

  // Unprimed (the very first evaluation, e.g. right after mount): whatever
  // is already waiting is already-there, not new, so it never opens the
  // bubble by itself. Reproduces as a page reload while an approval is
  // already pending popping the bubble open on its own.
  assert.equal(companionShouldOpen(false, emptyKeys, pendingLines), false)

  // Primed, with nothing seen yet: the same lines are now a new wait.
  assert.equal(companionShouldOpen(true, emptyKeys, pendingLines), true)

  const afterPending = companionWaitingKeys(pendingLines)
  // The same pending approval on the next tick is not new.
  assert.equal(companionShouldOpen(true, afterPending, pendingLines), false)

  // A running turn alone (no approval, no failure) never opens the bubble,
  // primed or not.
  const runningLines = companionState({ s1: { running: true, pending: 0, queued: 0, title: 't' } }, [], now, text).lines
  assert.equal(companionShouldOpen(false, new Set(), runningLines), false)
  assert.equal(companionShouldOpen(true, new Set(), runningLines), false)

  // A new failure on top of an already-seen pending approval reopens it,
  // once primed.
  const failures: CompanionFailure[] = [{ key: 'failure:new', label: 'New failure', at: now }]
  const mixedLines = companionState({ s1: { running: false, pending: 1, queued: 0, title: 't' } }, failures, now, text).lines
  assert.equal(companionShouldOpen(true, afterPending, mixedLines), true)
  // Unprimed, the same mixed lines still do not open — the mount guard
  // applies regardless of what is in prevWaitingKeys.
  assert.equal(companionShouldOpen(false, afterPending, mixedLines), false)
})

test('a bubble that opened itself closes itself once there is nothing left to show it for (#1194)', () => {
  const pendingLines = companionState({ s1: { running: false, pending: 1, queued: 0, title: 't' } }, [], now, text).lines
  const runningLines = companionState({ s1: { running: true, pending: 0, queued: 0, title: 't' } }, [], now, text).lines
  const noLines: CompanionLine[] = []

  // Self-opened (autoOpened: true) and the wait is answered/dismissed/
  // expired: nothing but a running line, or nothing at all, is left.
  assert.equal(companionShouldAutoClose(true, noLines), true)
  assert.equal(companionShouldAutoClose(true, runningLines), true)

  // Self-opened, but the wait that opened it (or another one) is still there.
  assert.equal(companionShouldAutoClose(true, pendingLines), false)

  // The user opened it by hand (autoOpened: false): never auto-closed,
  // whether or not anything is waiting.
  assert.equal(companionShouldAutoClose(false, noLines), false)
  assert.equal(companionShouldAutoClose(false, pendingLines), false)

  // A showing server line (#1192) keeps a self-opened bubble up even once
  // every approval/failure line has cleared, until it is itself dismissed
  // or expires — only then does the usual rule apply again.
  assert.equal(companionShouldAutoClose(true, noLines, true), false)
  assert.equal(companionShouldAutoClose(true, noLines, false), true)
  assert.equal(companionShouldAutoClose(true, noLines), true) // defaults to false
})

test('companion ask hands off the user words, with the guidance kept apart', () => {
  const handoff = companionHandoffForAsk('  what should I inspect?  ', 'agentruntime')
  // The message (and so the bubble and the session title) is the user's own words.
  assert.equal(handoff.prompt, 'what should I inspect?')
  assert.match(handoff.context, /TARS companion/i)
  assert.match(handoff.context, /agentruntime/i)
  assert.match(handoff.context, /do not run tools/i)
  assert.doesNotMatch(handoff.context, /what should I inspect/)

  const korean = companionHandoffForAsk('어디를 보면 돼?', 'pulse', 'ko')
  assert.equal(korean.prompt, '어디를 보면 돼?')
  assert.match(korean.context, /TARS 콘솔/)
  assert.match(korean.context, /\(pulse\)/)
  assert.doesNotMatch(korean.context, /어디를 보면 돼/)

  assert.equal(companionHandoffForAsk('x'.repeat(900), 'board').prompt.length <= 600, true)
  assert.match(appSource, /companionHandoffForAsk\(/)
  assert.match(appSource, /initialContext=\{aiContext\}/)
})

test('companion_enabled gates visibility from config values', () => {
  assert.equal(companionEnabledFromConfigValues({ companion_enabled: true }), true)
  assert.equal(companionEnabledFromConfigValues({ companion_enabled: false }), false)
  assert.equal(companionEnabledFromConfigValues(undefined), false)
})

// A leaf rule is "selector-list { declarations }". Scanning the whole text
// with this flat regex still isolates a rule nested one level inside a
// wrapper (@media, @keyframes): the wrapper's own "{" cannot be absorbed by
// the selector-list capture ([^{}]+), so the match only ever starts at the
// innermost selector list.
function companionLeafRules(text: string): { selectors: string[]; body: string }[] {
  return Array.from(text.matchAll(/([^{}]+)\{([^{}]*)\}/g)).map((rule) => ({
    selectors: rule[1].split(',').map((s) => s.trim()).filter(Boolean),
    body: rule[2],
  }))
}

function companionDeclarations(body: string): { prop: string; value: string }[] {
  return body
    .split(';')
    .map((d) => d.trim())
    .filter(Boolean)
    .map((d) => {
      const i = d.indexOf(':')
      return i < 0 ? null : { prop: d.slice(0, i).trim(), value: d.slice(i + 1).trim() }
    })
    .filter((d): d is { prop: string; value: string } => d !== null)
}

function companionSelectorsWhere(
  rules: { selectors: string[]; body: string }[],
  prop: string,
  matches: (value: string) => boolean,
): Set<string> {
  const set = new Set<string>()
  for (const rule of rules) {
    for (const decl of companionDeclarations(rule.body)) {
      if (decl.prop === prop && matches(decl.value)) for (const sel of rule.selectors) set.add(sel)
    }
  }
  return set
}

test('companion honours prefers-reduced-motion for every animated or transitioning selector', () => {
  const style = componentSource.slice(componentSource.indexOf('<style>'))
  const reducedStart = style.indexOf('@media (prefers-reduced-motion: reduce)')
  assert.ok(reducedStart > 0, 'reduced-motion block is missing')
  const outsideRules = companionLeafRules(style.slice(0, reducedStart))
  const reducedRules = companionLeafRules(style.slice(reducedStart))

  // "animation: none" / "transition: none" outside the reduced block is a
  // static override (e.g. an expression that skips the blink), not a
  // repeating animation — it is not required to appear in the reduced
  // block too.
  const animatedSelectors = companionSelectorsWhere(outsideRules, 'animation', (v) => v !== 'none')
  const transitioningSelectors = companionSelectorsWhere(outsideRules, 'transition', (v) => v !== 'none')
  const reducedAnimated = companionSelectorsWhere(reducedRules, 'animation', (v) => v === 'none')
  const reducedTransitioned = companionSelectorsWhere(reducedRules, 'transition', (v) => v === 'none')

  assert.ok(animatedSelectors.size > 0, 'expected at least one animated selector to check against')
  for (const sel of animatedSelectors) {
    assert.ok(reducedAnimated.has(sel), `reduced motion does not stop the animation on ${sel}`)
  }
  for (const sel of transitioningSelectors) {
    assert.ok(reducedTransitioned.has(sel), `reduced motion does not stop the transition on ${sel}`)
  }
})
