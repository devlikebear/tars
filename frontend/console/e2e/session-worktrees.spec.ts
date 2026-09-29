// Hybrid worktrees (#971): the first session to work in a repository holds
// its write lease; a second session that starts a turn there moves into a
// worktree of its own, and its edits reach the checkout only when applied.
// The mock LLM's [e2e:write3] turn calls write_file three times.

import { execFileSync } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const composer = (page: Page) => page.locator('.chat-main textarea')
// Native file edits ask first (#970); allowing write_file for the session
// lets the three writes through with one answer.
async function editFiles(page: Page) {
  await composer(page).fill('Edit the files [e2e:write3]')
  await composer(page).press('Enter')
  const pending = page.locator('.chat-log .approval:not(.settled)')
  await pending.getByRole('button', { name: 'Allow write_file for this session' }).click()
  await turnSettled(page)
}
const turnSettled = (page: Page) => expect(page.locator('.chat-form-actions button[type="submit"]')).toBeVisible()
const original = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

function git(dir: string, ...args: string[]) {
  execFileSync('git', ['-c', 'user.name=E2E', '-c', 'user.email=e2e@example.com', '-c', 'commit.gpgSign=false', ...args], { cwd: dir }) // NOSONAR: the machine's own git from PATH
}

async function newSessionIn(page: Page, dir: string): Promise<string> {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const id = page.url().split('/').pop() ?? ''
  expect((await page.request.put(`/v1/admin/sessions/${id}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })).ok()).toBe(true)
  await page.reload()
  return id
}

test('a second session in a busy repository works in its own worktree', async ({ page }) => {
  const repo = mkdtempSync(join(tmpdir(), 'tars-e2e-lease-'))
  git(repo, 'init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), original)
  git(repo, 'add', '-A')
  git(repo, 'commit', '-q', '-m', 'init')

  // Session A takes the lease with a plain turn.
  await newSessionIn(page, repo)
  await expect(page.getByTestId('worktree-isolate')).toBeVisible()
  await composer(page).fill('Hello from A')
  await composer(page).press('Enter')
  await turnSettled(page)
  await expect(page.getByTestId('worktree-chip')).toHaveCount(0)

  // Session B starts a turn in the same repository and is isolated.
  const b = await newSessionIn(page, repo)
  await editFiles(page)
  const chip = page.getByTestId('worktree-chip')
  await expect(chip).toContainText('worktree · 3 files')
  await expect(chip).toHaveAttribute('title', new RegExp(`tars/session-${b}`))
  expect(readFileSync(join(repo, 'base.txt'), 'utf8')).toBe(original)
  expect(existsSync(join(repo, 'notes.md'))).toBe(false)

  // Apply brings the edits into the checkout as uncommitted changes.
  await chip.click()
  await expect(page.getByText('Isolated because another session was working in this repository')).toBeVisible()
  await page.getByRole('button', { name: 'Apply to checkout' }).click()
  await expect(chip).toHaveCount(0)
  await expect(page.getByTestId('worktree-isolate')).toBeVisible()
  expect(readFileSync(join(repo, 'base.txt'), 'utf8')).toContain('line 19 edited')
  expect(existsSync(join(repo, 'notes.md'))).toBe(true)
  expect(execFileSync('git', ['branch', '--list', 'tars/*'], { cwd: repo }).toString()).toBe('') // NOSONAR: the machine's own git

  const audit = await (await page.request.get(`/v1/ops/automation-audit?session_id=${b}&limit=20`)).json()
  const results = (audit.items as { action: string; result: string }[]).filter((e) => e.action === 'session_worktree').map((e) => e.result)
  expect(results.toSorted((a, b) => a.localeCompare(b))).toEqual(['applied', 'isolated'])
})

test('isolate by hand, then discard', async ({ page }) => {
  const repo = mkdtempSync(join(tmpdir(), 'tars-e2e-isolate-'))
  git(repo, 'init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), original)
  git(repo, 'add', '-A')
  git(repo, 'commit', '-q', '-m', 'init')

  await newSessionIn(page, repo)
  await page.getByTestId('worktree-isolate').click()
  const chip = page.getByTestId('worktree-chip')
  await expect(chip).toHaveAttribute('title', /No changes yet/)
  await editFiles(page)
  await expect(chip).toContainText('3 files')

  await chip.click()
  await page.getByRole('button', { name: 'Discard', exact: true }).click()
  await page.getByRole('button', { name: 'Discard for good' }).click()
  await expect(chip).toHaveCount(0)
  expect(readFileSync(join(repo, 'base.txt'), 'utf8')).toBe(original)
  expect(existsSync(join(repo, 'notes.md'))).toBe(false)
})
