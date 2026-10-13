// Console presence (tars#1249): the console's own signal to the server
// that a person is actually at the console right now, as opposed to merely
// having something subscribed to /v1/events/stream (which stays open for
// hours on a tab left in the background, or the desktop tray's own
// subscription). The server's initiative loop reads this instead of the
// SSE broker's subscriber count to decide arrival and "at the console"
// (internal/initiative's ConsolePresentAt/ConsoleArrivedAt).
//
// Deliberately independent of the companion feature (CompanionPet.svelte):
// CASE can be turned off in config and the presence signal must still go,
// so this module is started from App.svelte directly, not from inside
// CompanionPet.
import { requestJSON } from './api/client.ts'

// PRESENCE_ARRIVAL_HIDDEN_MS matches CASE's own
// COMPANION_JUST_ARRIVED_HIDDEN_MS and the server's presenceTTL (10
// minutes) — a tab hidden for at least this long becoming visible again is
// an arrival worth signaling immediately, the same gap CASE's bubble uses
// to decide the same thing, and the gap the server treats a signal as
// stale after.
export const PRESENCE_ARRIVAL_HIDDEN_MS = 10 * 60 * 1000

// PRESENCE_SEND_INTERVAL_MS throttles a signal sent because of real input
// (click/key/pointer/scroll) to at most once every two minutes — the
// server only needs "still here" every couple of minutes, not on every
// keystroke, and presenceTTL (10 minutes) has four signals of slack before
// a silent console would read as gone.
export const PRESENCE_SEND_INTERVAL_MS = 2 * 60 * 1000

// presenceShouldSendOnVisible decides whether a tab becoming visible again
// is itself worth a signal: hiddenForMs is how long it was hidden just now.
export function presenceShouldSendOnVisible(
  hiddenForMs: number,
  thresholdMs = PRESENCE_ARRIVAL_HIDDEN_MS,
): boolean {
  return hiddenForMs >= thresholdMs
}

// presenceShouldSendOnStart decides whether starting the tracker should
// send a signal immediately: only when the tab is visible right now. A
// tab that starts hidden — the browser restoring a background tab, or a
// desktop shell window created hidden at login — must not send on open
// (the issue's "hidden tabs never send" rule applies to the very first
// signal too, not only later ones); it waits for its first
// visibilitychange instead, which always sends once it happens, since
// there is no "since when" to measure a hidden start's duration against.
export function presenceShouldSendOnStart(hidden: boolean): boolean {
  return !hidden
}

// presenceShouldSendOnInput throttles a real-input signal to at most once
// every intervalMs since the last signal actually sent (lastSentAt), of any
// kind — an arrival signal counts too, so input right after opening the
// console does not immediately send a second one.
export function presenceShouldSendOnInput(
  lastSentAt: number,
  now: number,
  intervalMs = PRESENCE_SEND_INTERVAL_MS,
): boolean {
  return now - lastSentAt >= intervalMs
}

let started = false
let lastSentAt = 0
let hiddenAt: number | null = null
// pendingFirstVisible is true when startConsolePresence ran while the tab
// was already hidden (see presenceShouldSendOnStart) — the next time it
// becomes visible counts as an arrival unconditionally, since hiddenAt
// has no meaningful start point to measure a duration from (the tab could
// have been hidden since before this page even loaded).
let pendingFirstVisible = false

function sendPresenceSignal() {
  lastSentAt = Date.now()
  // Best effort: a failed request is logged server-side (if it even
  // reaches the server) and the console tries again on the next
  // arrival/input signal. Nothing here is worth surfacing to the user.
  void requestJSON('/v1/console/presence', { method: 'POST' }).catch(() => {})
}

function handleVisibilityChange() {
  if (document.hidden) {
    hiddenAt = Date.now()
    return
  }
  const wasHiddenForMs = pendingFirstVisible ? Infinity : hiddenAt !== null ? Date.now() - hiddenAt : 0
  pendingFirstVisible = false
  hiddenAt = null
  if (presenceShouldSendOnVisible(wasHiddenForMs)) sendPresenceSignal()
}

function handleInput() {
  // Belt and suspenders: none of these listeners should fire while the
  // tab is hidden, but a hidden tab must never send regardless.
  if (document.hidden) return
  if (presenceShouldSendOnInput(lastSentAt, Date.now())) sendPresenceSignal()
}

// startConsolePresence wires the listeners and, when the tab is visible
// right now, sends one signal immediately — opening the console is itself
// an arrival, the same as CASE's own mount-time justArrived cue. A tab
// that starts hidden sends nothing yet (presenceShouldSendOnStart) and
// waits for its first visibilitychange instead. Idempotent: a second call
// while already started does nothing.
export function startConsolePresence() {
  if (started || typeof document === 'undefined') return
  started = true
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('pointerdown', handleInput, { passive: true })
  window.addEventListener('keydown', handleInput)
  window.addEventListener('pointermove', handleInput, { passive: true })
  window.addEventListener('wheel', handleInput, { passive: true })
  if (presenceShouldSendOnStart(document.hidden)) {
    sendPresenceSignal()
    return
  }
  pendingFirstVisible = true
}

export function stopConsolePresence() {
  if (!started) return
  started = false
  pendingFirstVisible = false
  hiddenAt = null
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('pointerdown', handleInput)
  window.removeEventListener('keydown', handleInput)
  window.removeEventListener('pointermove', handleInput)
  window.removeEventListener('wheel', handleInput)
}
