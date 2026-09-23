// Chat workbench E2E baseline (#968). Each test scripts a path that was
// verified by hand while the session store (#978) and the Chat.svelte split
// (#979) landed. The LLM is e2e/mock-llm.mjs, which echoes the last user
// message, so assistant text is deterministic.

import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const sessionItems = (page: Page) => page.locator('.dock-left .session-item')
const activeSessionItem = (page: Page) => page.locator('.dock-left .session-item.active')
// The header shows `.new-chat-title` until a session exists.
const sessionHeaderTitle = (page: Page) => page.locator('.chat-main .session-title:not(.new-chat-title)')
const lastAssistant = (page: Page) => page.locator('.chat-msg.chat-assistant').last()

async function send(page: Page, text: string) {
  await composer(page).fill(text)
  await composer(page).press('Enter')
}

async function newSession(page: Page): Promise<string> {
  const before = page.url()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).not.toHaveURL(before)
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  return page.url().split('/').pop() ?? ''
}

test.beforeEach(async ({ page }) => {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
})

test('a new session gets its own route, header, and sidebar entry', async ({ page }) => {
  const count = await sessionItems(page).count()
  const id = await newSession(page)
  expect(id).not.toBe('')
  await expect(sessionHeaderTitle(page)).toBeVisible()
  await expect(sessionItems(page)).toHaveCount(count + 1)
  await expect(activeSessionItem(page)).toHaveCount(1)
})

test('the first send on /console/chat streams a reply and adopts the session without remounting', async ({ page }) => {
  const count = await sessionItems(page).count()
  const input = await composer(page).elementHandle()

  await send(page, 'hello from e2e')
  await expect(lastAssistant(page)).toContainText('Echo: hello from e2e')

  // The route stays put; the store adopted the lazily created session.
  await expect(page).toHaveURL(/\/console\/chat$/)
  await expect(sessionHeaderTitle(page)).toBeVisible()
  await expect(sessionItems(page)).toHaveCount(count + 1)
  // Same composer element: ChatPanel did not remount mid-stream.
  expect(await input?.evaluate((el) => el.isConnected)).toBe(true)
  expect(await composer(page).evaluate((el, prev) => el === prev, input)).toBe(true)
})

test('switching sessions clears the draft and moves the sidebar highlight', async ({ page }) => {
  const first = await newSession(page)
  const second = await newSession(page)
  expect(second).not.toBe(first)

  await composer(page).fill('unsent draft')
  await page.locator(`.dock-left .session-item:not(.active) .session-btn`).first().click()

  await expect(page).not.toHaveURL(new RegExp(`${second}$`))
  await expect(composer(page)).toHaveValue('')
  await expect(activeSessionItem(page)).toHaveCount(1)
})

test('a streamed turn settles into the session: title, health, and history survive a reload', async ({ page }) => {
  await newSession(page)
  await send(page, 'persist me')
  await expect(lastAssistant(page)).toContainText('Echo: persist me')

  await page.reload()
  await expect(page.locator('.chat-msg.chat-user').last()).toContainText('persist me')
  await expect(lastAssistant(page)).toContainText('Echo: persist me')
  await expect(page.locator('.session-health-badge')).toBeVisible()
})

test('compact reports its result and reloads the thread', async ({ page }) => {
  await newSession(page)
  await send(page, 'one')
  await expect(lastAssistant(page)).toContainText('Echo: one')
  const input = await composer(page).elementHandle()

  await page.locator('.session-menu-trigger').click()
  await page.locator('.session-menu-popover button').nth(2).click()

  await expect(page.locator('.action-feedback')).toBeVisible()
  // The thread remounts, so the composer is a new element.
  await expect.poll(async () => input?.evaluate((el) => el.isConnected)).toBe(false)
})

test('the toolbar and the header health badge drive one dock layout', async ({ page }) => {
  await newSession(page)
  const gitToggle = page.locator('.pulse-toggle-btn', { hasText: 'Git' })

  await gitToggle.click()
  await expect(gitToggle).toHaveClass(/active/)
  await expect(page.locator('.dock-right')).toBeVisible()

  await page.locator('.session-health-badge').click()
  await expect(gitToggle).not.toHaveClass(/active/)
  await expect(page.locator('.dock-right')).toContainText(/session health/i)
})

test('a re-docked panel keeps its zone across a reload', async ({ page }) => {
  await newSession(page)
  await page.locator('.session-health-badge').click()
  await page.locator('.dock-right button[aria-label="Dock bottom"]').click()
  await expect(page.locator('.dock-bottom')).toContainText(/session health/i)

  await page.reload()
  await page.locator('.session-health-badge').click()
  await expect(page.locator('.dock-bottom')).toContainText(/session health/i)

  // Restore the default so later tests start from the usual layout.
  await page.locator('.dock-bottom button[aria-label="Dock right"]').click()
  await expect(page.locator('.dock-right')).toContainText(/session health/i)
})

test('the terminal keeps its xterm instance when moved between zones (#667)', async ({ page }) => {
  await newSession(page)
  await page.locator('.pulse-toggle-btn').nth(1).click() // Files
  await page.locator('.dock-right button[title^="Open integrated terminal"]').click()

  const terminal = page.locator('.dock-terminal')
  await expect(terminal).toHaveAttribute('data-zone', 'bottom')
  await expect(terminal.locator('.xterm')).toBeVisible()
  const xterm = await terminal.locator('.xterm').elementHandle()

  await terminal.locator('button[aria-label="Dock right"]').click()
  await expect(terminal).toHaveAttribute('data-zone', 'right')
  expect(await xterm?.evaluate((el) => el.isConnected)).toBe(true)
  expect(await terminal.locator('.xterm').evaluate((el, prev) => el === prev, xterm)).toBe(true)

  await terminal.locator('button[aria-label="Close panel"]').click()
})
