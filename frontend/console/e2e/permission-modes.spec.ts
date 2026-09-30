// Permission modes (#970): the status bar switches the session's tool
// permission mode, ⇧Tab cycles it, plan mode keeps a turn read-only with a
// bar to approve the plan, and accept edits lets file edits through
// without a card. Decisions land in the automation audit. The mock LLM's
// [e2e:write3] turn calls write_file three times.

import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const modeSelect = (page: Page) => page.getByTestId('status-permission').locator('select')
const approvals = (page: Page) => page.locator('.chat-log .approval')
const assistant = (page: Page) => page.locator('.chat-msg.chat-assistant')
const turnSettled = (page: Page) => expect(page.locator('.chat-form-actions button[type="submit"]')).toBeVisible()

const original = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

async function newSessionIn(page: Page): Promise<{ id: string; dir: string }> {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-modes-'))
  writeFileSync(join(dir, 'base.txt'), original)
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const id = page.url().split('/').pop() ?? ''
  const res = await page.request.put(`/v1/admin/sessions/${id}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })
  expect(res.ok()).toBe(true)
  return { id, dir }
}

test('plan mode keeps a turn read-only until the plan is approved', async ({ page }) => {
  const { id, dir } = await newSessionIn(page)

  // A native session inherits "ask".
  await expect(modeSelect(page)).toBeVisible()
  await expect(modeSelect(page).locator('option:checked')).toHaveText('Default (Ask)')
  await modeSelect(page).selectOption('plan')
  await expect(modeSelect(page)).toHaveValue('plan')

  // Plan: write_file is refused without a card and nothing changes on disk.
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await turnSettled(page)
  await expect(approvals(page)).toHaveCount(0)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toBe(original)

  // Approve the plan: the mode switches and the agent is told to go on.
  const bar = page.locator('.plan-bar')
  await expect(bar).toContainText('Plan mode: read-only')
  await bar.getByRole('button', { name: 'Approve plan · accept edits' }).click()
  await expect(modeSelect(page)).toHaveValue('accept_edits')
  await expect(assistant(page).filter({ hasText: 'Echo: The plan is approved. Go ahead and carry it out.' })).toHaveCount(1)
  await expect(bar).toHaveCount(0)

  // Accept edits: file edits run without a card.
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await turnSettled(page)
  await expect(approvals(page)).toHaveCount(0)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toContain('line 19 edited')

  // ⇧Tab in the composer cycles the mode.
  await composer(page).focus()
  await composer(page).press('Shift+Tab')
  await expect(modeSelect(page)).toHaveValue('plan')
  await composer(page).press('Shift+Tab')
  await expect(modeSelect(page)).toHaveValue('auto')

  // The audit has the plan-mode refusals and the mode changes.
  const audit = await (await page.request.get(`/v1/ops/automation-audit?session_id=${id}&limit=50`)).json()
  const results = (audit.items as { action: string; result: string }[]).map((e) => `${e.action}:${e.result}`)
  expect(results).toContain('chat_tool_permission:refused_plan_mode')
  expect(results.filter((r) => r === 'chat_permission_mode:changed').length).toBeGreaterThanOrEqual(4)
})

test('back to the default mode asks again', async ({ page }) => {
  const { dir } = await newSessionIn(page)
  await modeSelect(page).selectOption('auto')
  await expect(modeSelect(page)).toHaveValue('auto')
  await modeSelect(page).selectOption('')
  await expect(modeSelect(page).locator('option:checked')).toHaveText('Default (Ask)')

  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  // Each of the three writes asks; deny them all.
  for (let answered = 1; answered <= 3; answered++) {
    await expect(page.locator('.chat-log .approval:not(.settled)')).toHaveCount(1)
    await page.locator('.chat-log .approval:not(.settled)').getByRole('button', { name: 'Deny' }).click()
    await expect(page.locator('.chat-log .approval.settled')).toHaveCount(answered)
  }
  await turnSettled(page)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toBe(original)
})
