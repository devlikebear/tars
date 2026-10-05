// Session board (#971): the console home groups sessions by the folder they
// work in and shows, live, which one waits for input and what the latest
// turn changed. The mock LLM's [e2e:write3] command calls write_file, which
// asks for approval first (#970), so a turn can be held in "needs input".

import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const pendingApproval = (page: Page) => page.locator('.chat-log .approval:not(.settled)')

function seedProject(): string {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-board-'))
  const lines = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
  writeFileSync(join(dir, 'base.txt'), lines.join('\n') + '\n')
  return dir
}

async function newSessionIn(page: Page, dir: string, title: string): Promise<string> {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = page.url().split('/').pop() ?? ''
  const workdirs = await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, {
    data: { work_dirs: [dir], current_dir: dir },
  })
  expect(workdirs.ok()).toBe(true)
  const renamed = await page.request.patch(`/v1/admin/sessions/${sessionId}`, { data: { title } })
  expect(renamed.ok()).toBe(true)
  return sessionId
}

test('the board is home and follows a session from needs input to its change', async ({ page, context }) => {
  const dir = seedProject()
  const sessionId = await newSessionIn(page, dir, 'Board E2E')

  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await expect(pendingApproval(page)).toHaveCount(1)

  // The sidebar marks the waiting session.
  await expect(page.locator('.dock-left .live-status.needs-input')).toBeVisible()

  // Another tab on the console home.
  const board = await context.newPage()
  await board.goto('/console')
  await expect(board.getByRole('heading', { name: 'Sessions', level: 2 })).toBeVisible()
  const group = board.getByRole('region', { name: basename(dir) })
  const card = group.locator(`.session-card[data-session-id="${sessionId}"]`)
  await expect(card).toContainText('Board E2E')
  await expect(card).toContainText('needs input')
  await expect(card).toContainText('1 approval waiting')

  // The "Needs input" filter keeps only waiting sessions.
  await board.getByRole('button', { name: /^Needs input/ }).click()
  await expect(board.locator('.session-card')).toHaveCount(1)
  await board.getByRole('button', { name: /^All/ }).click()

  // Answer in the chat tab; the board follows.
  await pendingApproval(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.')
  await expect(card).not.toContainText('needs input', { timeout: 15_000 })
  await expect(card).toContainText('3 files', { timeout: 15_000 })
  await expect(card).toContainText('+6')

  // A card opens its chat.
  await card.click()
  await expect(board).toHaveURL(new RegExp(`/console/chat/${sessionId}$`))
  await board.close()
})

test('the system overview moved under System', async ({ page }) => {
  await page.goto('/console/system')
  const home = page.locator('.home')
  await expect(home.getByRole('heading', { name: 'Mission Control', level: 2 })).toBeVisible()
  for (const name of ['Recent notifications', 'Recommended actions', 'Delivery']) {
    await expect(home.getByRole('heading', { name, exact: true })).toBeVisible()
  }
  for (const name of ['Active plans', 'Agent runs', 'Cron jobs', 'Active sessions', 'Continue working on...']) {
    await expect(home.getByRole('heading', { name, exact: true })).toHaveCount(0)
  }
  await expect(home.locator('.status-strip .status-tile')).toHaveCount(7)
  const nav = page.getByRole('navigation', { name: 'Main navigation' })
  await expect(nav.getByRole('link', { name: 'Overview' })).toHaveClass(/active/)
  await nav.getByRole('link', { name: 'Sessions' }).click()
  await expect(page).toHaveURL(/\/console$/)
  await expect(page.getByRole('heading', { name: 'Sessions', level: 2 })).toBeVisible()
})
