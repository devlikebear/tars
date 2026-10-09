import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  companionEnabledFromConfigValues,
  companionFailureFromEvent,
  companionHandoffForAsk,
  companionShouldOpen,
  companionState,
  companionWaitingKeys,
  pruneFailures,
  shouldShowCompanion,
  type CompanionFailure,
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
  assert.match(appSource, /onNavigate=\{navigate\}/)
  assert.match(appSource, /onDismissFailure=\{handleDismissCompanionFailure\}/)
  assert.match(appSource, /onAsk=\{handleCompanionAsk\}/)
  assert.match(appSource, /routeView=\{route\.view\}/)
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

test('the companion text addresses the user, not itself', () => {
  assert.equal(companionEn.emptyLine, 'Nothing is waiting on you right now.')
  assert.equal(companionEn.badgeAria(3), '3 waiting on you')
  assert.doesNotMatch(companionEn.emptyLine, /waiting on me/)
  assert.doesNotMatch(companionEn.badgeAria(1), /waiting on me/)
})

test('an empty activity snapshot is the idle mood with no badge and no lines', () => {
  const snapshot = companionState({}, [], now, text)
  assert.deepEqual(snapshot, { mood: 'idle', badge: 0, lines: [] })
})

test('a pending chat approval is a warn-mood line to that session', () => {
  const activity: ActivityMap = { s1: { running: false, pending: 2, queued: 0, title: 'Refactor session' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(snapshot.mood, 'warn')
  assert.equal(snapshot.badge, 2)
  assert.deepEqual(snapshot.lines, [
    { key: 'pending:s1', kind: 'pending', label: text.pending(2, 'Refactor session'), path: '/console/chat/s1' },
  ])
})

test('a queued unattended approval is a warn-mood line to Ops', () => {
  const activity: ActivityMap = { s2: { running: false, pending: 0, queued: 1, title: 'Nightly cron' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(snapshot.mood, 'warn')
  assert.equal(snapshot.badge, 1)
  assert.deepEqual(snapshot.lines, [
    { key: 'queued:s2', kind: 'queued', label: text.queued(1, 'Nightly cron'), path: '/console/ops' },
  ])
})

test('a running turn alone is the focus mood with a line and no badge', () => {
  const activity: ActivityMap = { s3: { running: true, pending: 0, queued: 0, title: 'Long task' } }
  const snapshot = companionState(activity, [], now, text)
  assert.equal(snapshot.mood, 'focus')
  assert.equal(snapshot.badge, 0)
  assert.deepEqual(snapshot.lines, [
    { key: 'running:s3', kind: 'running', label: text.running('Long task'), path: '/console/chat/s3' },
  ])
})

test('a recent failure is the error mood, outranking warn and focus', () => {
  const failures: CompanionFailure[] = [{ key: 'failure:1', sessionId: 's4', label: 'Cron failed', at: now }]
  const activity: ActivityMap = { s4: { running: true, pending: 1, queued: 0, title: 'Mixed session' } }
  const snapshot = companionState(activity, failures, now, text)
  assert.equal(snapshot.mood, 'error')
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

test('companion honours prefers-reduced-motion for every animated selector', () => {
  const style = componentSource.slice(componentSource.indexOf('<style>'))
  const reducedStart = style.indexOf('@media (prefers-reduced-motion: reduce)')
  assert.ok(reducedStart > 0, 'reduced-motion block is missing')
  const reduced = style.slice(reducedStart)
  const reducedSelectors = new Set(
    (reduced.match(/\{([^{}]*)\{\s*animation:\s*none/)?.[1] ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
  )
  // A rule is "selector { ... animation: ... }" outside @keyframes and the reduced block.
  const animated = new Set<string>()
  for (const rule of style.slice(0, reducedStart).matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!/(^|;|\s)animation\s*:/.test(rule[2])) continue
    for (const sel of rule[1].split(',')) animated.add(sel.trim())
  }
  assert.ok(animated.size > 0)
  for (const sel of animated) {
    assert.ok(reducedSelectors.has(sel), `reduced motion does not stop ${sel}`)
  }
})
