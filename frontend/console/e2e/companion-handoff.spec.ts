// Companion handoff: asking the floating companion opens the full chat with
// the user's words in the composer. The companion's guidance (tone, console
// area, no tools) goes to the model as console_context, so the bubble, the
// session title and the board show only what the user typed.

import { expect, test } from '@playwright/test'

// The pet floats and nods; reduced motion keeps its button still enough for
// Playwright's stability check, however earlier specs made it react.
test.use({ contextOptions: { reducedMotion: 'reduce' } })

test('a companion question keeps its words as the message and the title', async ({ page }, testInfo) => {
  // A per-run suffix keeps --repeat-each runs from finding each other's sessions.
  const words = `which session needs me first? (run ${testInfo.repeatEachIndex}-${Date.now().toString(36)})`
  await page.goto('/console')
  await page.getByRole('button', { name: 'Talk to TARS companion' }).click()
  await page.getByRole('textbox', { name: 'Ask TARS companion' }).fill(words)
  await page.getByRole('button', { name: 'Send companion prompt' }).click()

  await expect(page).toHaveURL(/\/console\/chat/)
  const composer = page.locator('.chat-main textarea')
  await expect(composer).toHaveValue(words)
  await expect(page.locator('.console-context-chip')).toBeVisible()
  // The session list refreshes when the turn ends. Slow the title save so
  // it lands after a refresh that does not wait for it would have read the
  // list (seen on slow CI runners); the sidebar must still get the title.
  await page.route('**/v1/admin/sessions/*', async (route) => {
    if (route.request().method() === 'PATCH') await new Promise((resolve) => setTimeout(resolve, 300))
    await route.fallback()
  })
  await composer.press('Enter')

  // The model got the guidance: the mock names the console area it read.
  const reply = page.locator('.chat-log .chat-assistant').last()
  await expect(reply).toContainText(`Companion at the Console (board): ${words}`)
  await expect(page.locator('.console-context-chip')).toHaveCount(0)
  const bubble = page.locator('.chat-log .chat-user .chat-text').last()
  await expect(bubble).toHaveText(words)

  // The session is titled with the user's words, never the guidance.
  let sessionId = ''
  await expect.poll(async () => {
    const res = await page.request.get('/v1/admin/sessions')
    if (!res.ok()) return ''
    const body = await res.json()
    const list: { id: string; title: string }[] = Array.isArray(body) ? body : body.sessions ?? []
    const found = list.find((s) => s.title === words)
    sessionId = found?.id ?? ''
    return list.some((s) => s.title.includes('companion inside the Console')) ? 'guidance in a title' : sessionId ? 'found' : ''
  }).toBe('found')
  await expect(page.locator('.dock-left .session-title').filter({ hasText: words })).toHaveCount(1)

  // Reloaded from the transcript, the bubble is still only the user's words.
  await page.goto(`/console/chat/${sessionId}`)
  await expect(page.locator('.chat-log .chat-user .chat-text').last()).toHaveText(words)

  await page.goto('/console')
  const card = page.locator(`.session-card[data-session-id="${sessionId}"]`)
  await expect(card).toContainText(words)
  await expect(card).not.toContainText('companion inside the Console')
})

// The companion's bubble opens by itself on ops, cron, and usage events, so
// it must never sit on the chat rail: a covered rail icon cannot be clicked
// until the bubble goes (seen as a 45 s stall in workbench-ko after the
// unattended-approvals spec left cron and ops events behind).
test('the open companion bubble leaves every chat rail icon clickable', async ({ page }) => {
  await page.goto('/console/chat')
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await page.getByRole('button', { name: 'Talk to TARS companion' }).click()
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
