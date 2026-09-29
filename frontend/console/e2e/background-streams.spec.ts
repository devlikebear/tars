// Background streams (#971): a turn keeps running when the console leaves
// its session, and coming back attaches to it. The mock LLM's [e2e:write3]
// turn stops on a write_file approval, which holds it open while the test
// switches sessions.

import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const pending = (page: Page) => page.locator('.chat-log .approval:not(.settled)')
const original = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

test('leaving a session mid-turn and coming back picks the turn up', async ({ page }) => {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-background-'))
  writeFileSync(join(dir, 'base.txt'), original)
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const id = page.url().split('/').pop() ?? ''
  expect((await page.request.put(`/v1/admin/sessions/${id}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })).ok()).toBe(true)

  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await expect(pending(page)).toContainText('Allow write_file?')

  // Leave for another session: the turn keeps waiting on the server.
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).not.toHaveURL(new RegExp(id))
  await expect(pending(page)).toHaveCount(0)

  // Come back: the turn, its reply bubble and the open question return.
  await page.goto(`/console/chat/${id}`)
  await expect(pending(page)).toContainText('Allow write_file?')
  await expect(page.locator('.chat-form-actions button[type="submit"]')).toHaveCount(0)
  await pending(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').filter({ hasText: 'Wrote 3 files.' })).toHaveCount(1)
  await expect(page.locator('.chat-form-actions button[type="submit"]')).toBeVisible()
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toContain('line 19 edited')
})
