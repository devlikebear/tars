// Side session (#971): another session in a dock panel next to the active
// one. It sends to that session, shows its reply and answers its approval
// cards without switching. The mock LLM's [e2e:write3] turn calls write_file
// three times.

import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const original = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

// newSession waits for the URL to name a session other than the one open,
// so two calls in a row cannot read the same ID.
async function newSession(page: Page): Promise<string> {
  const before = page.url()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect.poll(() => page.url()).not.toBe(before)
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  return page.url().split('/').pop() ?? ''
}

test('work in a second session from the side panel', async ({ page }) => {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-side-'))
  writeFileSync(join(dir, 'base.txt'), original)
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  const sideId = await newSession(page)
  expect((await page.request.put(`/v1/admin/sessions/${sideId}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })).ok()).toBe(true)
  expect((await page.request.patch(`/v1/admin/sessions/${sideId}`, { data: { title: 'Side work' } })).ok()).toBe(true)
  const mainId = await newSession(page)
  expect(mainId).not.toBe(sideId)

  await page.getByRole('button', { name: 'Side session' }).first().click()
  const side = page.getByTestId('side-session')
  await expect(side).toBeVisible()
  await side.getByLabel('Session to show beside this one').selectOption(sideId)
  await side.locator('textarea').fill('Edit the files [e2e:write3]')
  await side.locator('textarea').press('Enter')

  const card = side.locator('.approval:not(.settled)')
  await expect(card).toContainText('Allow write_file?')
  await card.getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(side.locator('.side-assistant').filter({ hasText: 'Wrote 3 files.' })).toHaveCount(1)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toContain('line 19 edited')

  // The main session is untouched and still the one in the URL.
  expect(page.url()).toContain(mainId)
  await expect(page.locator('.chat-log .chat-msg.chat-user')).toHaveCount(0)

  // Open makes it the main session.
  await side.getByRole('button', { name: 'Open' }).click()
  await expect(page).toHaveURL(new RegExp(sideId))
})
