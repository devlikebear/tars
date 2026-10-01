// The session cwd: `/cwd <path>` registers an existing folder, and the Files
// panel and the status bar chip show the same active cwd.

import { mkdirSync, realpathSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const cwdChip = (page: Page) => page.getByRole('group', { name: 'Session status' }).locator('.cwd-chip')

async function newSession(page: Page) {
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await expect(cwdChip(page)).toBeVisible()
}

async function runSlash(page: Page, text: string) {
  await composer(page).fill(text)
  await composer(page).press('Enter')
}

// A folder outside the session's candidates, resolved the way the server
// stores it (macOS temp dirs sit behind a /private symlink).
function freshFolder(name: string): string {
  const workspace = process.env.TARS_E2E_WORKSPACE
  if (!workspace) throw new Error('TARS_E2E_WORKSPACE is not set')
  const dir = join(workspace, 'cwd-e2e', `${name}-${Date.now()}`)
  mkdirSync(dir, { recursive: true })
  return realpathSync(dir)
}

test.beforeEach(async ({ page }) => {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
})

test('/cwd <path> adds an existing folder and switches to it', async ({ page }) => {
  await newSession(page)
  const repo = freshFolder('repo')

  await runSlash(page, `/cwd ${repo}`)
  await expect(page.locator('.action-feedback')).toContainText('cwd →')
  await expect(cwdChip(page)).toHaveAttribute('title', repo)

  // The Files panel follows: its folder list now has the repo, selected.
  await page.locator('.chat-rail [data-panel="artifacts"]').click()
  await expect(page.locator('.dock-right .workdir-select')).toHaveValue(repo)
})

test('/cwd <path> reports a folder that does not exist', async ({ page }) => {
  await newSession(page)
  const before = await cwdChip(page).getAttribute('title')

  await runSlash(page, `/cwd ${join(freshFolder('parent'), 'missing')}`)
  await expect(page.locator('.action-feedback')).toContainText('does not exist')
  await expect(cwdChip(page)).toHaveAttribute('title', before ?? '')
})

test('picking a folder in the Files panel moves the status bar chip', async ({ page }) => {
  await newSession(page)
  const before = await cwdChip(page).getAttribute('title')

  await page.locator('.chat-rail [data-panel="artifacts"]').click()
  const panel = page.locator('.dock-right')
  await panel.locator('.workdir-bar button[title^="Add"]').click()
  // The picker shows a placeholder "/" until its start path loads; the
  // Select Here button stays disabled until then, so wait for it first.
  const selectHere = panel.getByRole('button', { name: 'Select Here' })
  await expect(selectHere).toBeEnabled()
  // The picker's path strip is an input since the typed-path picker.
  const picked = await panel.locator('.pick-current').inputValue()
  expect(picked).toBeTruthy()
  await selectHere.click()

  await expect(cwdChip(page)).not.toHaveAttribute('title', before ?? '')
  await expect(cwdChip(page)).toHaveAttribute('title', realpathSync(picked!.trim()))
})
