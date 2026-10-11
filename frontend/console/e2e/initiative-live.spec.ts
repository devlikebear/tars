// Initiative live mode, end to end (tars#1220): a greet/check_in decision
// composes 1-3 lines (the mock LLM's fixed text), writes a real assistant
// message to the main session, and tells CASE through the same companion
// event path #1192 built. initiative only ever advances here through the
// e2e-only tick route (internal/tarsserver/e2e_initiative.go, `-tags e2e`
// only — see webServers.ts) because the real ticker is set to 24h
// (playwright.config.ts) precisely so it never fires on its own mid-suite
// and interrupts an unrelated spec (the #1194 class of bug).

import { expect, test, type Page } from '@playwright/test'

const body = (page: Page) => page.locator('.companion-pet .companion-body')
const serverLine = (page: Page) => page.locator('.companion-line-server')

async function gotoAndWaitForStream(page: Page, path: string) {
  const streamResponse = page.waitForResponse((res) => res.url().includes('/v1/events/stream'), { timeout: 10_000 })
  await page.goto(path)
  await streamResponse
}

async function mainSessionId(page: Page): Promise<string> {
  const res = await page.request.get('/v1/chat/board')
  expect(res.ok()).toBeTruthy()
  const data = await res.json()
  const main = (data.sessions ?? []).find((s: { title?: string }) => s.title === 'main')
  expect(main, 'the server must have a main session').toBeTruthy()
  return main.id
}

type HistoryMessage = { role: string; content: string; initiative?: { intent: string; entry_id?: string } }

async function history(page: Page, sessionId: string): Promise<HistoryMessage[]> {
  const res = await page.request.get(`/v1/admin/sessions/${sessionId}/history`)
  expect(res.ok()).toBeTruthy()
  // An empty transcript marshals as JSON null (a nil Go slice), not [].
  return (await res.json()) ?? []
}

async function tick(page: Page, at: string) {
  const res = await page.request.post('/v1/e2e/initiative/tick', { data: { at } })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

const SPOKEN_TEXT = 'Welcome back! Hope the trip went well.'

// Relative to the real moment the test runs, not a hardcoded date: the
// observer only reads a session's transcript when it was touched within
// observerSessionHorizon (7 days) of the tick's own "now" — a tick dated
// years away would make every real session (including the one this test
// just wrote a message to) look untouched for 7 days and get skipped,
// leaving the last-user signal unknown. +5h/+1m/+25h all stay inside that
// window while still comfortably past the 4h arrival gap, the 5m cooldown
// configured for this suite (playwright.config.ts), and the 24h
// long-absence threshold. quiet_hours is configured as the degenerate
// "00:00-00:00" window specifically so these never need to dodge a real
// wall-clock hour.
const now = Date.now()
const TICK_1 = new Date(now + 5 * 60 * 60_000).toISOString() // +5h: an arrival after the message above
const TICK_2 = new Date(now + 5 * 60 * 60_000 + 60_000).toISOString() // +1m after tick 1: inside the cooldown
const TICK_3 = new Date(now + 30 * 60 * 60_000 + 60_000).toISOString() // +25h after tick 2: a long absence

test('live mode speaks once, shows it in the thread, and stays quiet on the very next tick', async ({ page }, testInfo) => {
  const sessionId = await mainSessionId(page)

  // Nothing before this test's own tick looks initiative-authored: the
  // real ticker (24h) cannot have fired during a suite that runs in
  // minutes, so only an explicit POST to the e2e tick route ever drives
  // this.
  const before = await history(page, sessionId)
  expect(before.some((m) => m.initiative)).toBe(false)

  // A real (side) message, not in the main session, gives the signal
  // deriver a known last-user timestamp — Go's own "unknown" default
  // reads as "just arrived" but deliberately not as "long absence" (the
  // asymmetry is intentional: silence with no record at all is not
  // evidence of absence), so the long-absence check-in tick below needs
  // one real data point to read from. The huge gap to the 2031 tick
  // timestamps below holds either way.
  await page.goto('/console/chat')
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sideComposer = page.locator('.chat-main textarea')
  await sideComposer.fill('hi there')
  await sideComposer.press('Enter')
  await expect(page.locator('.chat-msg.chat-assistant').last()).toBeVisible()

  // Viewing the board, not the main session itself, so the companion
  // event's line is free to open the bubble (P4's own-session rule only
  // suppresses it when the console is already looking at that session).
  await gotoAndWaitForStream(page, '/console')
  await expect(body(page)).toBeVisible()

  const entry1 = await tick(page, TICK_1)
  expect(entry1.speak).toBe(true)
  expect(entry1.delivery).toBe('delivered')

  await expect(serverLine(page)).toBeVisible()
  await expect(serverLine(page)).toHaveText(SPOKEN_TEXT)

  // Regression guard (PR #1229 review): the line must keep showing, not
  // just flash once past this one assertion. Wait past a full
  // sessionActivity poll cycle (4s, lib/stores/sessionActivity.svelte.ts)
  // — whose response can legitimately flip CASE's *expression* to
  // 'working' if some other session has a running turn
  // (lib/companion.ts companionExpression: a real running line always
  // outranks a cue for the face) — and past more than two of the
  // component's own 1s cue timer ticks (CompanionPet.svelte's cueTimer),
  // which is what actually re-evaluates the cue's 5-minute expiry
  // (COMPANION_SERVER_CUE_LINE_MS). Neither one may clear the bubble's
  // *line*: hasServerLine gates companionShouldAutoClose specifically so
  // a still-showing line survives both.
  await page.waitForTimeout(4500)
  await expect(serverLine(page)).toBeVisible()
  await expect(serverLine(page)).toHaveText(SPOKEN_TEXT)
  await page.screenshot({ path: testInfo.outputPath('initiative-bubble.png') })

  // Clicking the line goes to the main session and shows the same words,
  // marked as TARS having spoken first rather than replied.
  await serverLine(page).click()
  await expect(page).toHaveURL(new RegExp(`/console/chat/${sessionId}$`))
  const spokenMsg = page.locator('.chat-msg.chat-assistant').last()
  await expect(spokenMsg).toContainText(SPOKEN_TEXT)
  await expect(spokenMsg.locator('.chat-spoke-first-badge')).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('initiative-thread.png') })

  const afterTick1 = await history(page, sessionId)
  expect(afterTick1.length).toBe(before.length + 1)
  const spoken = afterTick1[afterTick1.length - 1]
  expect(spoken.initiative?.intent).toBe('greet')

  // One minute later is well inside the cooldown: no second compose call,
  // no second message.
  const entry2 = await tick(page, TICK_2)
  expect(entry2.speak).toBe(false)
  expect(entry2.reason).toBe('cooldown')
  const afterTick2 = await history(page, sessionId)
  expect(afterTick2.length).toBe(afterTick1.length)

  // The earlier click already cleared tick 1's line (companion-events.spec.ts
  // establishes that a server line's own click removes it) — confirm that
  // baseline before tick 3, so the same assertion right after it actually
  // means something.
  await expect(serverLine(page)).toHaveCount(0)

  // 25 hours later: past cooldown, read as a long-absence check-in. The
  // console is now looking at the main session itself, so the companion
  // event must not pop a new bubble line (P4's rule) — the new message
  // shows up in the thread on its own instead, through the same session
  // SSE refresh every other background actor already uses.
  const entry3 = await tick(page, TICK_3)
  expect(entry3.speak).toBe(true)
  expect(entry3.delivery).toBe('delivered')
  expect(entry3.intent).toBe('check_in')

  // The bubble must not have opened a new line for this one: session_id
  // on the companion event matches the session already on screen, so only
  // CASE's expression carries over (lib/companion.ts's own-session rule).
  await expect(serverLine(page)).toHaveCount(0)

  await expect(page.locator('.chat-msg.chat-assistant')).toHaveCount(2)
  const secondSpokenMsg = page.locator('.chat-msg.chat-assistant').last()
  await expect(secondSpokenMsg).toContainText(SPOKEN_TEXT)
  await expect(secondSpokenMsg.locator('.chat-spoke-first-badge')).toBeVisible()

  const afterTick3 = await history(page, sessionId)
  expect(afterTick3.length).toBe(afterTick2.length + 1)
})
