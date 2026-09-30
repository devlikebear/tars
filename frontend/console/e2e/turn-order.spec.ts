// A turn that talks between its tool calls reads the same after a reload
// as it did live: text, tool card, text. The server saves the turn in the
// order it streamed; before, a reopened turn showed its tool cards first
// and all of its text in one bubble after them (and a native provider's
// text before a tool call was not saved at all).
//
// The mock LLM's [e2e:narrate] command streams "Writing the note first.",
// calls write_file once, then answers "All written.".

import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const turnSettled = (page: Page) => expect(page.locator('.chat-form-actions button[type="submit"]')).toBeVisible()

// The thread's replies and tool cards, in document order.
async function threadOrder(page: Page): Promise<string[]> {
  return page.locator('.chat-log .chat-msg.chat-assistant, .chat-log .chat-tool').evaluateAll((nodes) =>
    nodes.map((node) =>
      node.classList.contains('chat-tool')
        ? `tool:${node.querySelector('.tool-header')?.textContent?.includes('write_file') ? 'write_file' : '?'}`
        : `text:${node.querySelector('.chat-text')?.textContent?.trim() ?? ''}`,
    ),
  )
}

const expected = ['text:Writing the note first.', 'tool:write_file', 'text:All written.']

test('a reopened turn keeps its text and tool cards in the order they streamed', async ({ page }) => {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-order-'))
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = page.url().split('/').pop() ?? ''
  const res = await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, {
    data: { work_dirs: [dir], current_dir: dir },
  })
  expect(res.ok()).toBe(true)

  await composer(page).fill('Write the note [e2e:narrate]')
  await composer(page).press('Enter')
  const approval = page.locator('.chat-log .approval:not(.settled)')
  await expect(approval).toHaveCount(1)
  await approval.getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('All written.')
  await turnSettled(page)
  await expect.poll(() => threadOrder(page)).toEqual(expected)

  await page.reload()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('All written.')
  await expect.poll(() => threadOrder(page)).toEqual(expected)
})
