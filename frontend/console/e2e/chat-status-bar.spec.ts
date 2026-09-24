// Chat header cleanup, panel rail, and composer status bar (#968).

import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const lastAssistant = (page: Page) => page.locator('.chat-msg.chat-assistant').last()
const statusBar = (page: Page) => page.getByRole('group', { name: 'Session status' })

async function newSession(page: Page) {
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await expect(statusBar(page).locator('.cwd-chip')).toBeVisible()
}

async function send(page: Page, text: string) {
  await composer(page).fill(text)
  await composer(page).press('Enter')
  await expect(lastAssistant(page)).toContainText(`Echo: ${text}`)
}

test.beforeEach(async ({ page }) => {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
})

test('the pulse strip is gone and panels live on the rail', async ({ page }) => {
  await expect(page.locator('.chat-pulse')).toHaveCount(0)
  const rail = page.getByRole('navigation', { name: 'Chat panels' })
  await expect(rail).toBeVisible()
  const git = rail.locator('[data-panel="git"]')
  await git.click()
  await expect(git).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.dock-right')).toBeVisible()
  await git.click()
  await expect(git).toHaveAttribute('aria-pressed', 'false')
})

test('the rail opens the command palette', async ({ page }) => {
  await page.getByRole('navigation', { name: 'Chat panels' }).locator('.rail-palette').click()
  await expect(page.getByRole('dialog', { name: 'Command palette' })).toBeVisible()
})

test('the status bar shows the cwd and this session’s usage after a turn', async ({ page }) => {
  await newSession(page)
  const sessionId = page.url().split('/').pop() ?? ''
  await send(page, 'count my tokens')
  await expect(statusBar(page).getByTestId('status-cost')).toBeVisible()

  // The session filter counts this session's calls and nobody else's.
  // (Token counts stay 0 here: the OpenAI-compatible client does not read
  // usage from streamed responses yet, so only calls are asserted.) The
  // server records a call after the stream ends, so poll for it.
  await expect.poll(async () => {
    const own = await page.request.get(`/v1/usage/summary?period=month&session_id=${sessionId}`)
    return (await own.json()).summary.total_calls
  }).toBeGreaterThan(0)
  const other = await page.request.get('/v1/usage/summary?period=month&session_id=no-such-session')
  expect((await other.json()).summary.total_calls).toBe(0)
})

test('a pinned tier applies to every turn, not only the first', async ({ page }) => {
  await newSession(page)
  await statusBar(page).locator('.tier-select').selectOption('heavy')
  await send(page, 'first turn')
  await expect(statusBar(page).getByTestId('status-served-by')).toContainText('heavy')

  await send(page, 'second turn')
  // Without the pin riding along, the second turn falls back to the default tier.
  await expect(statusBar(page).getByTestId('status-served-by')).toContainText('heavy')
})

test('choosing Auto releases the pin', async ({ page }) => {
  await newSession(page)
  const select = statusBar(page).locator('.tier-select')
  await select.selectOption('light')
  await expect(select).toHaveClass(/pinned/)
  await select.selectOption('auto')
  await expect(select).not.toHaveClass(/pinned/)
})
