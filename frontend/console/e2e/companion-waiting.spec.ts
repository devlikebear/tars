// CASE's "waiting on you" list across sessions (#1188 P5, #1193). A chat
// approval pending in a session other than the one on screen badges CASE
// and opens its bubble with a line to it, which clears once answered. The
// session the chat route is currently showing never sees its own pending
// approval in CASE (#1194 regression guard) — its own thread already shows
// the approval card. An empty wait is the empty-state line and the input
// only; a server-pushed failure is its own line, cleared by its own ×.
// companion-events.spec.ts covers the category "companion" server cue;
// this file covers the real pending/failure lines built from
// GET /v1/chat/activity and /v1/events/stream instead.

import { expect, test, type Page } from '@playwright/test'

// The pet floats (.companion-figure's companionFloat animation inside
// .companion-button); reduced motion keeps its button still enough for
// Playwright's own stability check on the click in the empty-state test
// below (same reason companion-handoff.spec.ts and companion-events.spec.ts
// set this before clicking the same button).
test.use({ contextOptions: { reducedMotion: 'reduce' } })

const badge = (page: Page) => page.locator('.companion-pet .companion-badge')
const bubble = (page: Page) => page.locator('.companion-bubble')

async function newChatSession(page: Page): Promise<string> {
  await page.goto('/console/chat')
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  return page.url().split('/').pop() ?? ''
}

// Opens a second session without a full page reload, so CASE (mounted at
// the App level, outside the router) stays primed across the switch — a
// pending key appearing only after that counts as new and opens the
// bubble by itself (lib/companion.ts companionShouldOpen).
async function openAnotherSession(page: Page): Promise<string> {
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  return page.url().split('/').pop() ?? ''
}

async function sendWrite3(page: Page): Promise<void> {
  const composer = page.locator('.chat-main textarea')
  await composer.fill('Edit the files [e2e:write3]')
  await composer.press('Enter')
}

type PendingApproval = { request_id: string; session_id: string }

async function pendingApprovalFor(page: Page, sessionId: string): Promise<PendingApproval> {
  let found: PendingApproval | undefined
  await expect(async () => {
    const activity = await (await page.request.get('/v1/chat/activity')).json()
    found = (activity.pending_approvals ?? []).find((p: PendingApproval) => p.session_id === sessionId)
    expect(found).toBeTruthy()
  }).toPass({ timeout: 15_000 })
  return found as PendingApproval
}

async function resolveApproval(page: Page, requestId: string, sessionId: string): Promise<void> {
  const res = await page.request.post(`/v1/chat/permissions/${requestId}`, {
    data: { session_id: sessionId, decision: 'allow_session' },
  })
  expect(res.ok()).toBe(true)
}

test('a pending approval in another session badges CASE, opens its bubble with a line, and navigates there on click', async ({ page }) => {
  const sessionA = await newChatSession(page)
  await sendWrite3(page)

  // Viewing a different session: A's card is not on this screen, so its
  // pending approval is CASE's, not the active thread's.
  await openAnotherSession(page)

  await expect(badge(page)).toHaveText('1', { timeout: 15_000 })
  await expect(bubble(page)).toBeVisible()
  const pendingLine = page.locator('.companion-line-pending')
  await expect(pendingLine).toBeVisible()

  await pendingLine.click()
  await expect(page).toHaveURL(new RegExp(`/console/chat/${sessionA}$`))

  // Finish the turn: a left-waiting approval would hold a chat slot on
  // the shared server for the specs after this one.
  const approval = await pendingApprovalFor(page, sessionA)
  await resolveApproval(page, approval.request_id, sessionA)
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.', { timeout: 15_000 })
})

test('answering the approval from another screen clears CASE\'s badge and line, and closes the bubble it opened itself', async ({ page }) => {
  const sessionA = await newChatSession(page)
  await sendWrite3(page)
  await openAnotherSession(page)

  await expect(badge(page)).toHaveText('1', { timeout: 15_000 })
  await expect(bubble(page)).toBeVisible()
  await expect(page.locator('.companion-line-pending')).toBeVisible()

  const approval = await pendingApprovalFor(page, sessionA)
  await resolveApproval(page, approval.request_id, sessionA)

  await expect(badge(page)).toHaveCount(0, { timeout: 15_000 })
  await expect(bubble(page)).toHaveCount(0, { timeout: 15_000 })
})

test('the session the chat route is showing does not see its own pending approval in CASE (#1194 regression)', async ({ page }) => {
  const sessionA = await newChatSession(page)

  // Registered before sending the message: page.waitForResponse only ever
  // sees future responses, and this has to be the real response CASE's own
  // background poll reads, not a separately issued check, to prove CASE
  // already had A's pending approval when it decided to show nothing.
  const sawOwnPending = page.waitForResponse(async (res) => {
    if (!res.url().includes('/v1/chat/activity')) return false
    try {
      const body = await res.json()
      return (body.pending_approvals ?? []).some((p: PendingApproval) => p.session_id === sessionA)
    } catch {
      return false
    }
  }, { timeout: 15_000 })

  await sendWrite3(page)
  await sawOwnPending

  await expect(badge(page)).toHaveCount(0)
  await expect(bubble(page)).toHaveCount(0)

  const approval = await pendingApprovalFor(page, sessionA)
  await resolveApproval(page, approval.request_id, sessionA)
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.', { timeout: 15_000 })
})

test('nothing waiting: opening the bubble shows only the empty-state line and the input, no badge', async ({ page }) => {
  await page.goto('/console/chat')
  await expect(badge(page)).toHaveCount(0)
  await expect(bubble(page)).toHaveCount(0)

  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await expect(bubble(page)).toBeVisible()
  await expect(page.locator('.companion-empty')).toBeVisible()
  await expect(page.locator('.companion-lines')).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: 'Ask CASE' })).toBeVisible()
  await expect(badge(page)).toHaveCount(0)
})

test('an error event is a failure line and badge, cleared by its own dismiss button (#1192 e2e event channel)', async ({ page }) => {
  await page.goto('/console/chat')
  await expect(badge(page)).toHaveCount(0)
  await expect(bubble(page)).toHaveCount(0)

  const res = await page.request.post('/v1/e2e/events', {
    data: {
      type: 'notification',
      category: 'ops',
      severity: 'error',
      title: 'Cron run failed',
      message: 'nightly backup',
      timestamp: new Date().toISOString(),
    },
  })
  expect(res.ok()).toBeTruthy()

  await expect(badge(page)).toHaveText('1')
  await expect(bubble(page)).toBeVisible()
  const failureLine = page.locator('.companion-line-failure')
  await expect(failureLine).toBeVisible()

  await page.locator('.companion-line-dismiss').click()
  await expect(failureLine).toHaveCount(0)
  await expect(badge(page)).toHaveCount(0)
  await expect(bubble(page)).toHaveCount(0)
})
