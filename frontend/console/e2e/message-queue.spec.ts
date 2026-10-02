// Follow-up queue (#971): while a turn runs, the composer queues instead of
// sending; the queue sends by itself when the turn ends, and "Send now"
// stops the running turn to send a message next. The mock LLM's
// [e2e:write3] turn waits on a write_file approval, which holds it open.

import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const pendingApproval = (page: Page) => page.locator('.chat-log .approval:not(.settled)')
const queue = (page: Page) => page.locator('.message-queue')
const assistant = (page: Page) => page.locator('.chat-msg.chat-assistant')

function seedProject(): string {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-queue-'))
  writeFileSync(join(dir, 'base.txt'), Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n')
  return dir
}

async function holdTurn(page: Page): Promise<void> {
  const dir = seedProject()
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = page.url().split('/').pop() ?? ''
  const res = await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })
  expect(res.ok()).toBe(true)
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await expect(pendingApproval(page)).toHaveCount(1)
}

test('messages typed during a turn queue up and send when it ends', async ({ page }) => {
  await holdTurn(page)

  await expect(composer(page)).toHaveAttribute('placeholder', /Queue a follow-up/)
  await composer(page).fill('follow up one')
  await composer(page).press('Enter')
  await expect(queue(page)).toContainText('1 queued message')
  await expect(queue(page)).toContainText('follow up one')
  await expect(composer(page)).toHaveValue('')

  await composer(page).fill('follow up two')
  await page.getByRole('button', { name: 'Queue', exact: true }).click()
  await expect(queue(page)).toContainText('2 queued messages')

  // Edit takes the second one back into the composer; queue it again.
  await queue(page).locator('.queue-item').nth(1).getByRole('button', { name: 'Edit' }).click()
  await expect(composer(page)).toHaveValue('follow up two')
  await expect(queue(page)).toContainText('1 queued message')
  await composer(page).fill('follow up two, edited')
  await composer(page).press('Enter')
  await expect(queue(page)).toContainText('2 queued messages')

  // A slash command picked from the menu mid-turn queues too, instead of
  // cutting the running turn off.
  await composer(page).fill('/comp')
  await expect(page.locator('.slash-popover')).toBeVisible()
  await composer(page).press('Enter')
  await expect(queue(page)).toContainText('3 queued messages')
  await expect(queue(page)).toContainText('/compact')
  await expect(pendingApproval(page)).toHaveCount(1)
  await queue(page).locator('.queue-item').nth(2).getByRole('button', { name: 'Remove' }).click()
  await expect(queue(page)).toContainText('2 queued messages')

  // The turn ends; both follow-ups go out in order, one turn each.
  await pendingApproval(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(assistant(page).filter({ hasText: 'Wrote 3 files.' })).toHaveCount(1)
  await expect(assistant(page).filter({ hasText: 'Echo: follow up one' })).toHaveCount(1)
  await expect(assistant(page).filter({ hasText: 'Echo: follow up two, edited' })).toHaveCount(1)
  await expect(queue(page)).toHaveCount(0)
})

test('Send now stops the running turn and sends that message next', async ({ page }) => {
  await holdTurn(page)
  await composer(page).fill('first in line')
  await composer(page).press('Enter')
  await composer(page).fill('jump the line')
  await composer(page).press('Enter')

  // The stopped turn's stream can end, and the next turn start, before the
  // cancel request answers (seen on slow CI runners). Hold the answer until
  // then so this ordering is the one tested: finishing the cancel must not
  // stop the turn that took over.
  const aborted: string[] = []
  page.on('requestfailed', (req) => {
    if (req.method() === 'POST' && new URL(req.url()).pathname === '/v1/chat') aborted.push(req.postData() ?? '')
  })
  await page.route((url) => url.pathname === '/v1/chat/cancel', async (route) => {
    const nextTurn = page.waitForRequest((req) => req.method() === 'POST' && new URL(req.url()).pathname === '/v1/chat')
    const response = await route.fetch()
    await nextTurn
    await route.fulfill({ response })
  })
  await queue(page).locator('.queue-item').nth(1).getByRole('button', { name: 'Send now' }).click()
  await expect(assistant(page).filter({ hasText: 'Echo: jump the line' })).toHaveCount(1)
  await expect(assistant(page).filter({ hasText: 'Echo: first in line' })).toHaveCount(1)
  await expect(queue(page)).toHaveCount(0)
  // Neither queued turn was cut off on the way (a late abort once hit the
  // turn that took over, leaving its reply blank and the next one running
  // beside it).
  expect(aborted.filter((body) => /"message":"(first in line|jump the line)"/.test(body))).toEqual([])
})

test('Stop pauses the queue until it is resumed', async ({ page }) => {
  await holdTurn(page)
  await composer(page).fill('after the stop')
  await composer(page).press('Enter')

  // The stopped turn ends when its stream does, with the server's
  // `cancelled`. The console once aborted the stream as soon as the cancel
  // answered, while the server was still winding the turn down: Resume then
  // sent into a session still held (409) and the message was lost.
  const aborted: string[] = []
  page.on('requestfailed', (req) => {
    if (req.method() === 'POST' && new URL(req.url()).pathname === '/v1/chat') aborted.push(req.postData() ?? '')
  })
  await page.locator('.chat-form-actions').getByRole('button', { name: 'Stop' }).click()
  await expect(queue(page)).toContainText('Paused')
  expect(aborted).toEqual([])
  await expect(assistant(page).filter({ hasText: 'Echo: after the stop' })).toHaveCount(0)

  await queue(page).getByRole('button', { name: 'Resume' }).click()
  await expect(assistant(page).filter({ hasText: 'Echo: after the stop' })).toHaveCount(1)
  await expect(queue(page)).toHaveCount(0)
})
