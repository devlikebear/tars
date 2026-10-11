// CASE's bubble question-and-answer (tars#1236): asking the floating
// companion no longer opens a new session — it sends a normal chat turn
// to the *main* session (light tier, this turn only) and streams the
// answer into the bubble itself, without leaving the screen the user was
// on. A long answer is cut in the bubble; "Continue in chat" opens the
// main session, where the full turn is already there. Asking while the
// main session is busy elsewhere refuses to send.

import { expect, test, type Page } from '@playwright/test'

// The pet floats; reduced motion keeps its button still enough for
// Playwright's stability check.
test.use({ contextOptions: { reducedMotion: 'reduce' } })

async function sessionList(page: Page): Promise<{ id: string; title: string; kind?: string; tier_pin?: string }[]> {
  const res = await page.request.get('/v1/admin/sessions')
  const body = await res.json()
  return Array.isArray(body) ? body : body.sessions ?? []
}

async function mainSessionId(page: Page): Promise<string> {
  const list = await sessionList(page)
  const main = list.find((s) => s.kind === 'main')
  if (!main) throw new Error('no main session found')
  return main.id
}

async function historyOf(page: Page, sessionId: string): Promise<{ role: string; content: string }[]> {
  const res = await page.request.get(`/v1/admin/sessions/${sessionId}/history`)
  return res.json()
}

test('asking CASE from the board answers in the bubble: no new session, the main session own turn, light tier', async ({ page }, testInfo) => {
  // Long enough (well under companionAskContext's 600-char clip) that the
  // mock's echoed reply ("Companion at ...: <words>") exceeds the bubble's
  // truncation length, so the same turn also exercises "Continue in chat".
  const words = `${testInfo.repeatEachIndex}-${Date.now()}-${'x'.repeat(500)}`

  await page.goto('/console')
  const sessionsBefore = await sessionList(page)
  const mainId = await mainSessionId(page)
  const mainBefore = (await sessionList(page)).find((s) => s.id === mainId)

  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await page.getByRole('textbox', { name: 'Ask CASE' }).fill(words)
  const chatRequest = page.waitForRequest((req) => req.url().includes('/v1/chat') && req.method() === 'POST')
  await page.getByRole('button', { name: 'Send to CASE' }).click()

  // No screen change (tars#1236 requirement 1).
  await expect(page).toHaveURL(/\/console\/?$/)

  // This turn's own request: the main session, light tier, no pin — the
  // session's tier_pin (checked again below) must stay whatever it was.
  const body = (await chatRequest).postDataJSON() as {
    session_id?: string
    tier_recommendation?: { chosen_tier?: string; pin?: boolean }
  }
  expect(body.session_id).toBe('main')
  expect(body.tier_recommendation?.chosen_tier).toBe('light')
  expect(body.tier_recommendation?.pin).toBeFalsy()

  await expect(page.locator('.companion-qa-question')).toHaveText(words)
  await expect(page.locator('.companion-qa-answer')).toContainText('Companion at', { timeout: 15_000 })
  const link = page.getByRole('button', { name: 'Continue in chat' })
  await expect(link).toBeVisible({ timeout: 15_000 })

  // The main session got a real user/assistant pair; no new session exists.
  // The checkpoint/history write can lag a beat behind the bubble's own
  // streamed text (which only waits on its own fetch), so retry.
  await expect(async () => {
    const history = await historyOf(page, mainId)
    const userMsg = history.findLast((m) => m.role === 'user')
    const assistantMsg = history.findLast((m) => m.role === 'assistant')
    expect(userMsg?.content).toContain(words)
    expect(assistantMsg?.content).toContain(`Companion at the Console (board): ${words}`)
  }).toPass({ timeout: 15_000 })

  const sessionsAfter = await sessionList(page)
  expect(sessionsAfter.length).toBe(sessionsBefore.length)

  const mainAfter = (await sessionList(page)).find((s) => s.id === mainId)
  expect(mainAfter?.tier_pin ?? '').toBe(mainBefore?.tier_pin ?? '')

  // "Continue in chat" opens the main session; the same question/answer
  // is already there (the full answer — only the bubble truncated it).
  await link.click()
  await expect(page).toHaveURL(new RegExp(`/console/chat/${mainId}$`))
  await expect(page.locator('.chat-log .chat-user .chat-text').last()).toHaveText(words)
  await expect(page.locator('.chat-log .chat-assistant').last()).toContainText(
    `Companion at the Console (board): ${words}`,
    { timeout: 15_000 },
  )
})

test('CASE does not send while the main session is busy, and keeps the typed question', async ({ page }) => {
  const mainId = await mainSessionId(page)

  // Faking GET /v1/chat/activity (what sessionActivity.svelte.ts polls,
  // and what isMainSessionBusy reads) rather than starting a real turn on
  // the shared main session: this run's other specs (initiative-live.spec.ts)
  // read that session's own history/thread and do not expect this test's
  // traffic in it.
  await page.route('**/v1/chat/activity', async (route) => {
    const res = await route.fetch()
    const body = await res.json()
    body.running = [
      ...(body.running ?? []),
      { session_id: mainId, session_title: 'main', started_at: new Date().toISOString() },
    ]
    await route.fulfill({ response: res, json: body })
  })

  // A fresh navigation re-mounts the app so its activity poll picks up the
  // faked-busy main session immediately, instead of waiting on the next
  // 4s timer tick.
  const activityResponse = page.waitForResponse((res) => res.url().includes('/v1/chat/activity'))
  await page.goto('/console')
  await activityResponse

  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await page.getByRole('textbox', { name: 'Ask CASE' }).fill('are you there?')
  await page.getByRole('button', { name: 'Send to CASE' }).click()

  await expect(page.locator('.companion-qa-error')).toContainText('busy', { timeout: 10_000 })
  await expect(page.getByRole('textbox', { name: 'Ask CASE' })).toHaveValue('are you there?')
  await expect(page).toHaveURL(/\/console\/?$/)

  // Nothing was actually sent — the real server never saw a turn either.
  const history = await historyOf(page, mainId)
  expect(history.some((m) => m.content.includes('are you there?'))).toBe(false)
})

// CASE's bubble opens by itself on a new approval wait or a new failure, so
// it must never sit on the chat rail: a covered rail icon cannot be clicked
// until the bubble goes (seen as a 45 s stall in workbench-ko after the
// unattended-approvals spec left cron and ops events behind).
test('the open companion bubble leaves every chat rail icon clickable', async ({ page }) => {
  await page.goto('/console/chat')
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await expect(page.locator('.companion-bubble')).toBeVisible()

  const covered = await page.locator('.chat-rail button').evaluateAll((buttons) =>
    buttons
      .filter((b) => {
        const r = b.getBoundingClientRect()
        const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
        return !(hit && b.contains(hit))
      })
      .map((b) => b.getAttribute('data-panel') ?? b.getAttribute('aria-label') ?? '?'),
  )
  expect(covered).toEqual([])
  await page.locator('.chat-rail [data-panel="health"]').click({ timeout: 2_000 })
  await expect(page.locator('.dock-right')).toBeVisible()
})
