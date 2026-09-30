// Unattended approvals (#970): a cron run of a session in Ask mode cannot
// show approval cards, so each tool call it would ask about is queued in the
// ops approvals and waits. The open session shows a needs-input bar for it,
// and the answer decides whether the call runs. The mock LLM's [e2e:write3]
// turn calls write_file three times.

import { existsSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

const original = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

test('a cron run in Ask mode waits for the needs-input bar', async ({ page }) => {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-unattended-'))
  writeFileSync(join(dir, 'base.txt'), original)
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const id = page.url().split('/').pop() ?? ''
  expect((await page.request.put(`/v1/admin/sessions/${id}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })).ok()).toBe(true)
  expect((await page.request.put(`/v1/admin/sessions/${id}/permission-mode`, { data: { mode: 'manual' } })).ok()).toBe(true)

  const job = await (
    await page.request.post('/v1/cron/jobs', {
      data: { name: 'e2e unattended', prompt: 'Edit the files [e2e:write3]', schedule: 'every:24h', enabled: false, session_id: id },
    })
  ).json()
  // The run blocks until every question is answered.
  const run = page.request.post(`/v1/cron/jobs/${job.id}/run`, { timeout: 60_000 })

  const bar = page.getByTestId('needs-input')
  const answer = async (button: 'Approve' | 'Reject', preview: string) => {
    await expect(bar).toHaveCount(1, { timeout: 15_000 })
    await expect(bar).toContainText('A cron run needs input')
    await expect(bar).toContainText(preview)
    await bar.getByRole('button', { name: button }).click()
    await expect(bar).toHaveCount(0, { timeout: 15_000 })
  }
  await answer('Approve', 'base.txt')
  await answer('Reject', 'notes.md')
  await answer('Approve', 'src/app.txt')

  expect((await run).ok()).toBe(true)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toContain('line 19 edited')
  expect(existsSync(join(dir, 'notes.md'))).toBe(false)
  expect(existsSync(join(dir, 'src', 'app.txt'))).toBe(true)

  const audit = await (await page.request.get(`/v1/ops/automation-audit?session_id=${id}&limit=50`)).json()
  const results = (audit.items as { actor: string; action: string; result: string }[])
    .filter((e) => e.action === 'chat_tool_permission')
    .map((e) => `${e.actor}:${e.result}`)
  expect(results.toSorted((a, b) => a.localeCompare(b))).toEqual(['ops:allowed', 'ops:allowed', 'ops:denied'])

  // The Ops page keeps the answered questions with where they came from.
  await page.goto('/console/ops')
  await expect(page.getByTestId('tool-approval').first()).toBeVisible()
})
