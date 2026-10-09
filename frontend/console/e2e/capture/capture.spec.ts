// README / tars-site media capture (`make console-screenshots`). One spec,
// run against the real `tars serve` + mock LLM (playwright.capture.config.ts),
// that walks the console in demo order — session board, a chat exchange, an
// approval, a Focus pipeline, the system overview, Settings — taking a named
// screenshot at each stop. Playwright records the whole run as video
// (config `use.video`), so the same walk also produces the raw footage for
// the tars-site demo clip; `scripts/process_captures.sh` (run by
// `make console-screenshots`, not here) converts the PNGs to webp under
// docs/screenshots/ and trims/encodes the recorded .webm into the mp4/webm/
// poster the site uses.
//
// Not a correctness check: no behavioral assertions beyond "the thing we're
// about to screenshot is actually on screen". Excluded from
// playwright.config.ts (testIgnore) and from CI.

import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page } from '@playwright/test'

const rawDir = fileURLToPath(new URL('./output/raw', import.meta.url))
mkdirSync(rawDir, { recursive: true })

async function shot(page: Page, name: string, dwellMs = 300) {
  // A settle so streamed/animated UI (toasts, card transitions) is not
  // caught mid-motion, and so the recorded video lingers on each screen
  // instead of jump-cutting between them.
  await page.waitForTimeout(dwellMs)
  await page.screenshot({ path: join(rawDir, `${name}.png`) })
}


function git(dir: string, ...args: string[]) {
  execFileSync('git', ['-c', 'user.name=E2E', '-c', 'user.email=e2e@example.com', '-c', 'commit.gpgSign=false', ...args], { cwd: dir }) // NOSONAR: the machine's own git from PATH
}

function newRepo(prefix: string): string {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), prefix)))
  git(repo, 'init', '-q', '-b', 'main')
  const lines = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
  writeFileSync(join(repo, 'base.txt'), lines.join('\n') + '\n')
  git(repo, 'add', '-A')
  git(repo, 'commit', '-q', '-m', 'init')
  return repo
}

const composer = (page: Page) => page.locator('.chat-main textarea')
const lastReply = (page: Page) => page.locator('.chat-msg.chat-assistant').last()

async function newSessionIn(page: Page, dir: string, title: string): Promise<string> {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const sessionId = decodeURIComponent(page.url().split('/').pop() ?? '')
  expect((await page.request.put(`/v1/admin/sessions/${sessionId}/workdirs`, { data: { work_dirs: [dir], current_dir: dir } })).ok()).toBe(true)
  expect((await page.request.patch(`/v1/admin/sessions/${sessionId}`, { data: { title } })).ok()).toBe(true)
  return sessionId
}

test('capture: board, chat, approval, focus, overview, settings', async ({ page }) => {
  // 1. Session board — the console landing page.
  await page.goto('/console')
  await expect(page.getByRole('heading', { name: 'Sessions', level: 2 })).toBeVisible()
  await shot(page, 'console-board')

  // 2. Chat — a short, visually complete exchange (text, then a tool card).
  // Auto mode so the note's write_file call runs without an approval card —
  // that demo belongs to the Approvals screenshot, in its own session.
  const chatDir = newRepo('tars-capture-chat-')
  const chatId = await newSessionIn(page, chatDir, 'Release notes draft')
  expect((await page.request.put(`/v1/admin/sessions/${chatId}/permission-mode`, { data: { mode: 'auto' } })).ok()).toBe(true)
  await composer(page).fill('Write a short note about today\'s progress. [e2e:narrate]')
  await composer(page).press('Enter')
  // The first turn of a session asks which tier to use instead of sending.
  const standardTier = page.getByRole('button', { name: 'Standard' })
  if (await standardTier.isVisible({ timeout: 2_000 }).catch(() => false)) {
    await standardTier.click()
  }
  await expect(lastReply(page)).toContainText('All written.', { timeout: 20_000 })
  await shot(page, 'console-chat', 1_500)

  // 3. Approvals (Ops) — the review queue and what feeds it. Session-scoped
  // tool-call approvals stay inline in the chat transcript (.chat-log
  // .approval) rather than listing here; this page covers cleanup plans,
  // queued unattended approvals, and the automation audit.
  await page.goto('/console/approvals')
  await expect(page.getByRole('heading', { name: 'Approvals review queue' })).toBeVisible({ timeout: 15_000 })
  await shot(page, 'console-approvals')

  // 4. Focus — a plan gate approved into an active build step.
  const focusDir = newRepo('tars-capture-focus-')
  await page.goto('/console/focus')
  await expect(page.getByTestId('focus-home')).toBeVisible()
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(focusDir)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  // The scenario markers must be in the text for the mock LLM to answer
  // correctly, but the Focus header shows this goal verbatim (truncated) —
  // put the readable description first so the screenshot's title isn't led
  // by test markers.
  await page.getByTestId('focus-new-goal').fill('Add a friendly startup banner [e2e:focus-plan] [e2e:focus-loop]')
  await page.getByTestId('focus-new-start').click()
  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  const focusId = decodeURIComponent(page.url().split('/').pop() ?? '')
  expect((await page.request.put(`/v1/admin/sessions/${focusId}/permission-mode`, { data: { mode: 'auto' } })).ok()).toBe(true)
  const gate = page.locator('[data-testid="focus-card"][data-kind="gate"]')
  await expect(gate).toBeVisible({ timeout: 15_000 })
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active', { timeout: 15_000 })
  await shot(page, 'console-focus', 1_500)

  // 5. System overview ("Mission Control").
  await page.goto('/console/system')
  await expect(page.locator('.home').getByRole('heading', { name: 'Mission Control', level: 2 })).toBeVisible()
  await shot(page, 'console-overview', 1_200)

  // 6. Settings — scrolled past Remote Access to the Quick Start grid. The
  // Remote Access card (RemoteAccessCard, `.remote-card`) shells out to the
  // *real* `tailscale status` on whatever machine runs this capture and
  // would leak that machine's device name and tailnet hostname into a
  // screenshot meant for a public README/site — it must never be on screen
  // for the shot, so scroll its bottom edge above the viewport first.
  await page.goto('/console/config')
  await expect(page.getByRole('textbox', { name: 'Search settings' })).toBeVisible()
  await expect(page.locator('.quick-start-card').first()).toBeVisible()
  // Remote Access detects Tailscale asynchronously and grows once it
  // resolves; measure its height only after that settles, or the scroll
  // below is computed too short and leaves part of it on screen.
  await expect(page.getByText('Checking Tailscale status...')).toHaveCount(0, { timeout: 15_000 })
  await page.waitForTimeout(300)
  // .config-page scrolls internally; the window itself does not.
  await page.evaluate(() => {
    const remote = document.querySelector('.remote-card')
    const container = document.querySelector('.config-page')
    if (remote && container) {
      container.scrollTop += remote.getBoundingClientRect().bottom - container.getBoundingClientRect().top + 8
    }
  })
  await expect(page.locator('.remote-card')).not.toBeInViewport()
  await shot(page, 'console-settings', 600)
})
