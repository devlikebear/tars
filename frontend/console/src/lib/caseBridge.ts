// CASE's Bridge (case-companion phase 2): the pure parts of the panel —
// where the user is asking from, what of a streaming answer to show, and
// which follow-up questions to offer. CaseBridge.svelte and
// stores/caseBridgeStore.svelte.ts use them; tests/caseBridge.test.ts
// checks them.

import type { CaseBridgeCard, CaseBridgeView } from './api/caseBridge.ts'

// The part of the console route the Bridge reads. App.svelte's route has
// more; only these matter here.
export type CaseBridgeRoute = { view: string; sessionId?: string }

// caseBridgeView is where a question is asked from: the route's name and,
// when it shows one (a chat or a focus pipeline), its session.
export function caseBridgeView(route: CaseBridgeRoute, title?: string): CaseBridgeView {
  const view: CaseBridgeView = { route: route.view || 'board' }
  if ((route.view === 'chat' || route.view === 'focus') && route.sessionId) {
    view.session_id = route.sessionId
    if (title?.trim()) view.title = title.trim()
  }
  return view
}

// canScopeToScreen: "this screen" only means something on a screen that
// shows one session or pipeline.
export function canScopeToScreen(view: CaseBridgeView): boolean {
  return !!view.session_id
}

const CARDS_OPEN = '<case-cards>'

// caseBridgeVisibleText is what of a streaming answer to show: the card
// block (which the server takes out of the final answer anyway) never
// flashes on screen, not even while its opening tag is half-streamed.
export function caseBridgeVisibleText(streamed: string): string {
  const at = streamed.indexOf(CARDS_OPEN)
  if (at >= 0) return streamed.slice(0, at).trimEnd()
  for (let n = Math.min(CARDS_OPEN.length - 1, streamed.length); n > 0; n--) {
    if (streamed.endsWith(CARDS_OPEN.slice(0, n))) return streamed.slice(0, -n).trimEnd()
  }
  return streamed
}

export type CaseBridgeFollowUp = 'stuck' | 'missed' | 'focusWaiting' | 'cronWhy' | 'approvalsOldest' | 'usageTop' | 'pulseWhat'

const FOLLOW_UP_FOR_KIND: Record<CaseBridgeCard['kind'], CaseBridgeFollowUp | null> = {
  focus: 'focusWaiting',
  session: null,
  cron: 'cronWhy',
  approvals: 'approvalsOldest',
  usage: 'usageTop',
  pulse: 'pulseWhat',
}

// caseBridgeFollowUps offers up to three next questions: one per kind of
// card the last answer pointed at, then the general openers.
export function caseBridgeFollowUps(cards: readonly CaseBridgeCard[]): CaseBridgeFollowUp[] {
  const out: CaseBridgeFollowUp[] = []
  for (const card of cards) {
    const next = FOLLOW_UP_FOR_KIND[card.kind]
    if (next && !out.includes(next)) out.push(next)
  }
  for (const opener of ['stuck', 'missed'] as const) {
    if (!out.includes(opener)) out.push(opener)
  }
  return out.slice(0, 3)
}
