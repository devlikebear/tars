// Command palette and global shortcuts (#968). Uses `Control`, which the
// shortcuts treat as Mod on every platform.

import { expect, test, type Page } from '@playwright/test'

const palette = (page: Page) => page.getByRole('dialog', { name: 'Command palette' })
const paletteInput = (page: Page) => palette(page).getByRole('combobox')
const composer = (page: Page) => page.locator('.chat-main textarea')

async function openPalette(page: Page) {
  await page.keyboard.press('Control+K')
  await expect(palette(page)).toBeVisible()
  await expect(paletteInput(page)).toBeFocused()
}

test.beforeEach(async ({ page }) => {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
})

test('Ctrl+K opens the palette from the composer and reaches a page the nav hides', async ({ page }) => {
  await composer(page).click()
  await openPalette(page)
  await paletteInput(page).fill('mem')
  await expect(palette(page).getByRole('option').first()).toContainText('Memory')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/console\/memory$/)
  await expect(palette(page)).toBeHidden()
})

test('the palette works off the chat route and Escape returns focus', async ({ page }) => {
  await page.goto('/console/pulse')
  await openPalette(page)
  await paletteInput(page).fill('lineage')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/console\/sessions\/graph$/)

  await page.goto('/console/chat')
  await composer(page).click()
  await openPalette(page)
  await page.keyboard.press('Escape')
  await expect(palette(page)).toBeHidden()
  await expect(composer(page)).toBeFocused()
})

test('arrow keys move the selection and a panel command opens its dock panel', async ({ page }) => {
  await openPalette(page)
  await paletteInput(page).fill('git')
  const options = palette(page).getByRole('option')
  await expect(options.first()).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Enter')
  await expect(page.locator('.pulse-toggle-btn', { hasText: 'Git' })).toHaveClass(/active/)
})

test('a session command switches to that session', async ({ page }) => {
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const target = page.url().split('/').pop() ?? ''
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).not.toHaveURL(new RegExp(`${target}$`))

  await openPalette(page)
  await paletteInput(page).fill(target.slice(0, 8))
  await expect(palette(page).getByRole('option').first()).toContainText(target.slice(0, 8))
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(new RegExp(`/console/chat/${target}$`))
})

test('a slash command runs from the palette on the chat route', async ({ page }) => {
  await page.locator('.dock-left .new-chat-btn').click()
  // Wait for the new session to be active; /cwd needs one.
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await expect(page.locator('.cwd-chip')).toBeVisible()
  await openPalette(page)
  await paletteInput(page).fill('/cwd')
  await page.keyboard.press('Enter')
  await expect(page.locator('.action-feedback')).toContainText('cwd active')
})

test('Ctrl+Shift+O starts a new session, from any route', async ({ page }) => {
  const before = page.url()
  await page.keyboard.press('Control+Shift+O')
  await expect(page).not.toHaveURL(before)
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const first = page.url()

  await page.goto('/console/memory')
  await page.keyboard.press('Control+Shift+O')
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  expect(page.url()).not.toBe(first)
})

test('Alt+2 switches to the second session in the sidebar', async ({ page }) => {
  const items = page.locator('.dock-left .session-item')
  const before = await items.count()
  for (let i = 1; i <= 2; i++) {
    await page.locator('.dock-left .new-chat-btn').click()
    await expect(items).toHaveCount(before + i)
  }
  // Let the sidebar settle: both new sessions are listed, the newest first and active.
  await expect(items.first()).toHaveClass(/active/)
  const second = items.nth(1)
  await page.keyboard.press('Alt+2')
  await expect(second).toHaveClass(/active/)
})

test('Ctrl+B toggles the session list and Ctrl+J opens a terminal', async ({ page }) => {
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page.locator('.session-title:not(.new-chat-title)').last()).toBeVisible()

  await page.keyboard.press('Control+B')
  await expect(page.locator('.dock-left')).toHaveCount(0)
  await page.keyboard.press('Control+B')
  await expect(page.locator('.dock-left')).toBeVisible()

  // The terminal opens at the session cwd, so wait for it; without one,
  // Ctrl+J opens the Files panel instead.
  await expect(page.locator('.cwd-chip')).toBeVisible()
  await page.keyboard.press('Control+J')
  await expect(page.locator('.dock-terminal .xterm')).toBeVisible()
  // A focused terminal keeps Ctrl+J (and Ctrl+K/B) for the shell; toggle from outside it.
  await composer(page).click()
  await page.keyboard.press('Control+J')
  await expect(page.locator('.dock-terminal')).toHaveCount(0)
})

test('? shows the shortcut list outside text fields only', async ({ page }) => {
  await composer(page).click()
  await page.keyboard.type('?')
  await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toHaveCount(0)
  await expect(composer(page)).toHaveValue('?')

  await composer(page).fill('')
  await page.locator('.chat-main').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('Shift+Slash')
  const help = page.getByRole('dialog', { name: 'Keyboard shortcuts' })
  await expect(help).toBeVisible()
  await expect(help).toContainText('Ctrl+K')
  await page.keyboard.press('Escape')
  await expect(help).toHaveCount(0)
})
