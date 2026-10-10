// CASE's server-driven cue (#1192): a category "companion" SSE event
// (expression, optional line, optional session_id) makes CASE show that
// expression and, when there is a line, a self-opening bubble line that
// navigates to the session on click. This spec posts real events to the
// real server's broker through POST /v1/e2e/events
// (internal/tarsserver/e2e_events.go, built only with `-tags e2e` —
// tarsServeWebServer in webServers.ts runs the e2e server with that tag)
// and watches the real /v1/events/stream carry them to the page, rather
// than faking the SSE response in the browser.

import { expect, test, type Page } from '@playwright/test'

test.use({ contextOptions: { reducedMotion: 'reduce' } })

const body = (page: Page) => page.locator('.companion-pet .companion-body')
const bubble = (page: Page) => page.locator('.companion-bubble')

// `page.waitForResponse` only ever sees *future* responses, so the
// listener has to be registered before the navigation that triggers the
// SSE connection, not after — otherwise, if the stream connected already,
// the wait would hang until the test's own timeout.
async function gotoAndWaitForStream(page: Page, path: string) {
  const streamResponse = page.waitForResponse((res) => res.url().includes('/v1/events/stream'), { timeout: 10_000 })
  await page.goto(path)
  await streamResponse
}

async function postCompanionEvent(
  page: Page,
  overrides: { expression: string; message?: string; session_id?: string; timestamp?: string },
) {
  const response = await page.request.post('/v1/e2e/events', {
    data: {
      type: 'notification',
      category: 'companion',
      severity: 'info',
      title: '',
      message: overrides.message ?? '',
      timestamp: overrides.timestamp ?? new Date().toISOString(),
      expression: overrides.expression,
      session_id: overrides.session_id,
    },
  })
  expect(response.ok()).toBeTruthy()
}

// Creates a session via the real "New Chat" button and returns its id,
// with the SSE stream already connected (the initial /console/chat load
// is the only full page navigation here; the click is an in-app route
// change that keeps the same connection).
async function newChatSessionId(page: Page): Promise<string> {
  await gotoAndWaitForStream(page, '/console/chat')
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  return page.url().split('/').pop() ?? ''
}

test('a companion event with a line changes CASE\'s face, opens the bubble above the waiting lines, and navigates on click', async ({ page }, testInfo) => {
  const targetSessionId = await newChatSessionId(page)

  await gotoAndWaitForStream(page, '/console')
  // Every mount greets first (#1191's justArrived cue, a few seconds);
  // this retries past that before the real assertions below.
  await expect(body(page)).toHaveClass(/expr-neutral/)
  await expect(bubble(page)).toHaveCount(0)

  const headerBadge = page.locator('.header-badge-btn .badge-count')
  const unreadBefore = (await headerBadge.count()) > 0 ? await headerBadge.textContent() : null

  await postCompanionEvent(page, {
    expression: 'alert',
    message: 'a session needs you over there',
    session_id: targetSessionId,
  })

  await expect(body(page)).toHaveClass(/expr-alert/)
  const line = page.locator('.companion-line-server')
  await expect(line).toBeVisible()
  await expect(line).toHaveText('a session needs you over there')
  // Distinct from the "waiting on you" line kinds — CASE talking, not a
  // queue item.
  await expect(line).not.toHaveClass(/companion-line-pending|companion-line-failure|companion-line-queued|companion-line-running/)

  await page.screenshot({ path: testInfo.outputPath('companion-server-line.png') })

  // CASE's own face/bubble is not a general notification: the header's
  // unread count never moved.
  const unreadAfter = (await headerBadge.count()) > 0 ? await headerBadge.textContent() : null
  expect(unreadAfter).toBe(unreadBefore)

  await line.click()
  await expect(page).toHaveURL(new RegExp(`/console/chat/${targetSessionId}$`))
  await expect(page.locator('.companion-line-server')).toHaveCount(0)
})

test('an expression outside COMPANION_EXPRESSIONS is ignored, the whole event included', async ({ page }) => {
  await gotoAndWaitForStream(page, '/console')
  await expect(body(page)).toHaveClass(/expr-neutral/)

  await postCompanionEvent(page, { expression: 'curious', message: 'should never show' })
  await page.waitForTimeout(300)

  await expect(body(page)).toHaveClass(/expr-neutral/)
  await expect(bubble(page)).toHaveCount(0)
})

test('a body_only companion event (no line) changes only the face — no bubble, no badge', async ({ page }) => {
  await gotoAndWaitForStream(page, '/console')
  await expect(body(page)).toHaveClass(/expr-neutral/)

  await postCompanionEvent(page, { expression: 'happy' })

  await expect(body(page)).toHaveClass(/expr-happy/)
  await expect(bubble(page)).toHaveCount(0)
  await expect(page.locator('.companion-badge')).toHaveCount(0)
})

test('a companion event older than 60s is ignored, as if it were a reconnect replay', async ({ page }) => {
  await gotoAndWaitForStream(page, '/console')
  await expect(body(page)).toHaveClass(/expr-neutral/)

  const stale = new Date(Date.now() - 61_000).toISOString()
  await postCompanionEvent(page, { expression: 'upset', message: 'too old to show', timestamp: stale })
  await page.waitForTimeout(300)

  await expect(body(page)).toHaveClass(/expr-neutral/)
  await expect(bubble(page)).toHaveCount(0)
})

test('viewing the event\'s own session: the face changes but the bubble never opens for it', async ({ page }) => {
  const sessionId = await newChatSessionId(page)
  await expect(body(page)).toHaveClass(/expr-neutral/)

  // That session's own thread already shows this message — only the face
  // carries over.
  await postCompanionEvent(page, { expression: 'wary', message: 'already visible in this thread', session_id: sessionId })

  await expect(body(page)).toHaveClass(/expr-wary/)
  await expect(bubble(page)).toHaveCount(0)
  await expect(page.locator('.companion-badge')).toHaveCount(0)
})

test('closing the bubble dismisses the server line for good, not just the view', async ({ page }) => {
  await gotoAndWaitForStream(page, '/console')
  await expect(body(page)).toHaveClass(/expr-neutral/)

  await postCompanionEvent(page, { expression: 'greeting', message: 'no session_id on this one' })

  const line = page.locator('.companion-line-server')
  await expect(line).toBeVisible()
  // No session_id: a static line (companion-line-server-static), not a
  // button — only closing dismisses it.
  await expect(line).toHaveClass(/companion-line-server-static/)

  await page.locator('.companion-close').click()
  await expect(bubble(page)).toHaveCount(0)

  // Reopening by hand shows no leftover line: the close dismissed the cue
  // itself, not merely the bubble's visibility.
  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await expect(page.locator('.companion-line-server')).toHaveCount(0)
})
