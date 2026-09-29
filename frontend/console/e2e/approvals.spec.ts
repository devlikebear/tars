// "Always allow" (#970): a rule chosen on an approval card is remembered for
// the session's folder by TARS — not in the project — so a new session there
// runs the tool without asking, and Session Config → Permissions removes it.
// The mock LLM's [e2e:write3] command calls write_file three times.

import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const approvals = (page: Page) => page.locator('.chat-log .approval')
const pendingApproval = (page: Page) => page.locator('.chat-log .approval:not(.settled)')
const lastReply = (page: Page) => page.locator('.chat-msg.chat-assistant').last()

function seedProject(): string {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-approvals-'))
  const lines = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
  writeFileSync(join(dir, 'base.txt'), lines.join('\n') + '\n')
  return dir
}

async function newSessionIn(page: Page, dir: string): Promise<void> {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = page.url().split('/').pop() ?? ''
  const res = await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, {
    data: { work_dirs: [dir], current_dir: dir },
  })
  expect(res.ok()).toBe(true)
}

async function sendWrite3(page: Page): Promise<void> {
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
}

test('always allow remembers the rule for the folder until it is removed', async ({ page }) => {
  const dir = seedProject()
  await newSessionIn(page, dir)
  await sendWrite3(page)
  await expect(pendingApproval(page)).toHaveCount(1)
  await expect(pendingApproval(page)).toBeFocused()
  await page.keyboard.press('a')
  await expect(lastReply(page)).toContainText('Wrote 3 files.')
  await expect(approvals(page)).toHaveCount(1)
  await expect(approvals(page)).toContainText('Always allowed in this folder: write_file')

  // A new session in the same folder writes without asking.
  await newSessionIn(page, dir)
  await sendWrite3(page)
  await expect(lastReply(page)).toContainText('Wrote 3 files.')
  await expect(approvals(page)).toHaveCount(0)

  // Session Config → Permissions lists the rule and removes it.
  await page.locator('.chat-rail [data-panel="config"]').click()
  await page.locator('.dock-right .config-tab', { hasText: 'Permissions' }).click()
  const row = page.locator('.dock-right .rules-row', { hasText: 'write_file' })
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: 'Remove' }).click()
  await expect(row).toHaveCount(0)

  // With the rule gone, the next write asks again.
  await newSessionIn(page, dir)
  await sendWrite3(page)
  await expect(pendingApproval(page)).toHaveCount(1)
})
