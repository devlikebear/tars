// Turn changes (#969): a turn that edits files in the session's folder shows
// a change card under it and fills the Changes panel. The mock LLM's
// [e2e:write3] command calls write_file three times: base.txt changed in two
// places (lines 2 and 19), plus notes.md and src/app.txt.
//
// write_file is a high-risk tool, so each call first asks through an
// approval card in the thread (#970).

import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
const card = (page: Page) => page.locator('.chat-log .turn-changes').last()
const changesPanel = (page: Page) => page.locator('.dock-right .changes-panel')
const approvals = (page: Page) => page.locator('.chat-log .approval')
const pendingApproval = (page: Page) => page.locator('.chat-log .approval:not(.settled)')
// A turn is over when the Send button is back: the reply can show before
// the turn's end snapshot is taken and the reply is saved.
const turnSettled = (page: Page) => expect(page.locator('.chat-form-actions button[type="submit"]')).toBeVisible()

// The base.txt the mock edits: "line 1" to "line 20".
function seedProject(): string {
  const dir = mkdtempSync(join(tmpdir(), 'tars-e2e-project-'))
  const lines = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
  writeFileSync(join(dir, 'base.txt'), lines.join('\n') + '\n')
  return dir
}

async function newSessionIn(page: Page, dir: string): Promise<string> {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = page.url().split('/').pop() ?? ''
  const res = await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, {
    data: { work_dirs: [dir], current_dir: dir },
  })
  expect(res.ok()).toBe(true)
  return sessionId
}

test('a turn that edits three files shows its diff in the thread and the Changes panel', async ({ page }) => {
  const dir = seedProject()
  const sessionId = await newSessionIn(page, dir)

  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')

  // One card for the first write; allowing write_file for the session lets
  // the other two through without asking.
  await expect(pendingApproval(page)).toHaveCount(1)
  await expect(pendingApproval(page)).toContainText('Allow write_file?')
  await expect(pendingApproval(page)).toContainText('base.txt')
  await pendingApproval(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.')
  await expect(approvals(page)).toHaveCount(1)
  await expect(approvals(page)).toContainText('Allowed for this session: write_file')
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).toContain('line 19 edited')

  // The card under the turn: collapsed summary, then every file's diff.
  await expect(card(page)).toContainText('3 files +6 −2')
  await card(page).locator('.turn-changes-toggle').click()
  await expect(card(page).locator('.turn-file')).toHaveCount(3)
  await expect(card(page).locator('.turn-file-path')).toHaveText(['base.txt', 'notes.md', 'src/app.txt'])
  await expect(card(page)).toContainText('line 2 edited')
  await expect(card(page)).toContainText('line 19 edited')

  // Open in Changes: the panel shows the turn, its files, and one file's diff.
  await card(page).getByRole('button', { name: 'Open in Changes' }).click()
  await expect(changesPanel(page)).toBeVisible()
  await expect(changesPanel(page).locator('.turn-row')).toHaveCount(1)
  await expect(changesPanel(page).locator('.turn-row')).toContainText('Edit the files')
  await expect(changesPanel(page).locator('.file-row')).toHaveCount(3)
  await changesPanel(page).locator('.file-row', { hasText: 'notes.md' }).click()
  await expect(changesPanel(page).locator('.diff-table')).toContainText('Written by the E2E agent.')
  await changesPanel(page).getByRole('button', { name: 'Session so far' }).click()
  await expect(changesPanel(page).locator('.file-row')).toHaveCount(3)

  // History keeps the card: the turn's user message carries its ID.
  await turnSettled(page)
  await page.reload()
  await expect(page).toHaveURL(new RegExp(`${sessionId}$`))
  await expect(card(page)).toContainText('3 files +6 −2')
})

test('denied writes leave the files alone and the turn still finishes', async ({ page }) => {
  const dir = seedProject()
  await newSessionIn(page, dir)
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')

  // Nothing is remembered on deny, so each of the three writes asks. The
  // card takes focus, so the n key answers it.
  for (let answered = 1; answered <= 3; answered++) {
    await expect(pendingApproval(page)).toHaveCount(1)
    await expect(pendingApproval(page)).toBeFocused()
    await page.keyboard.press('n')
    await expect(page.locator('.chat-log .approval.settled')).toHaveCount(answered)
  }
  // The mock counts tool results, denials included; the turn did not stop.
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.')
  await expect(approvals(page).filter({ hasText: 'Denied' })).toHaveCount(3)
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).not.toContain('edited')
  await expect(page.locator('.chat-log .turn-changes')).toHaveCount(0)
})

test('a turn without file changes gets no card', async ({ page }) => {
  await newSessionIn(page, seedProject())
  await composer(page).fill('just talk')
  await composer(page).press('Enter')
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Echo: just talk')
  await expect(page.locator('.chat-log .turn-changes')).toHaveCount(0)
})

test('one hunk is reverted after a confirm, and Undo puts it back', async ({ page }) => {
  const dir = seedProject()
  await newSessionIn(page, dir)
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await pendingApproval(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.')
  await turnSettled(page)
  await card(page).getByRole('button', { name: 'Open in Changes' }).click()
  const panel = changesPanel(page)
  await panel.locator('.file-row', { hasText: 'base.txt' }).click()

  // base.txt has two hunks: line 2 (h0) and line 19 (h1). Revert the second.
  const hunkButtons = panel.getByRole('button', { name: 'Revert hunk' })
  await expect(hunkButtons).toHaveCount(2)
  await hunkButtons.nth(1).click()
  const bar = panel.locator('.revert-bar')
  await expect(bar).toContainText('Revert 1 hunk in base.txt?')
  await bar.getByRole('button', { name: 'Revert', exact: true }).click()
  await expect(bar).toContainText('Reverted 1 file.')

  const base = () => readFileSync(join(dir, 'base.txt'), 'utf8')
  expect(base()).toContain('line 2 edited')
  expect(base()).not.toContain('line 19 edited')
  await expect(panel.locator('.reverted-badge')).toHaveCount(1)

  await bar.getByRole('button', { name: 'Undo' }).click()
  await expect(bar).toContainText('The revert is undone.')
  expect(base()).toContain('line 19 edited')
  await expect(panel.locator('.reverted-badge')).toHaveCount(0)
})

// #969's completion criterion, end to end on one page: a turn changes three
// files; the diff is reviewed; one hunk is reverted; a comment on another
// asks for rework; the next message carries both notes to the agent.
test('review a turn: revert a hunk, comment on another, and send them back', async ({ page }) => {
  const dir = seedProject()
  await newSessionIn(page, dir)
  const url = page.url()
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  await pendingApproval(page).getByRole('button', { name: 'Allow write_file for this session' }).click()
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Wrote 3 files.')
  await turnSettled(page)
  await expect(card(page)).toContainText('3 files +6 −2')

  await card(page).getByRole('button', { name: 'Open in Changes' }).click()
  const panel = changesPanel(page)
  await panel.locator('.file-row', { hasText: 'base.txt' }).click()
  await panel.getByRole('button', { name: 'Revert hunk' }).nth(1).click()
  await panel.locator('.revert-bar').getByRole('button', { name: 'Revert', exact: true }).click()
  await expect(panel.locator('.revert-bar')).toContainText('Reverted 1 file.')
  expect(readFileSync(join(dir, 'base.txt'), 'utf8')).not.toContain('line 19 edited')

  // The reverted hunk shows its mark; the other takes a comment.
  await panel.getByRole('button', { name: 'Comment', exact: true }).click()
  await panel.locator('.note-form textarea').fill('Say line two louder')
  await panel.getByRole('button', { name: 'Add note' }).click()
  const chips = page.locator('.chat-main .review-note-chip')
  await expect(chips).toHaveCount(2)
  await expect(chips.nth(0)).toContainText('base.txt (h1): reverted')
  await expect(chips.nth(1)).toContainText('base.txt (h0): Say line two louder')

  await composer(page).fill('Please rework')
  await composer(page).press('Enter')
  await expect(page.locator('.chat-msg.chat-assistant').last()).toContainText('Rework: Say line two louder')
  await expect(chips).toHaveCount(0)
  expect(page.url()).toBe(url)

  // The thread keeps the notes, folded under the message.
  await turnSettled(page)
  await page.reload()
  const sent = page.locator('.chat-msg.chat-user').last()
  await expect(sent).toContainText('Please rework')
  await expect(sent.locator('.review-notes-fold summary')).toHaveText('Review notes (2)')
})
