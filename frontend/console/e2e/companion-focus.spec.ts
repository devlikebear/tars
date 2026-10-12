// CASE on the focus screens (tars#1247): the companion now shows there too
// (the app sidebar stays hidden). The focus pipeline screen's own session
// (and its hidden Q&A session) counts as "already on screen" the same way
// the chat route's active session always has — their own pending/running
// lines are left out of CASE, while another session's still show. Every
// focus screen reserves CASE's own corner on the right so its button and
// bubble never cover a gate/finding/Q&A/new-task/template button, including
// a bubble that opens itself while a gate is open.

import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Locator, type Page } from '@playwright/test'

test.use({ contextOptions: { reducedMotion: 'reduce' } })

const planGoal = '[e2e:focus-plan] [e2e:focus-report] Add a greeting'
const reviewGoal = '[e2e:focus-plan] [e2e:focus-review] Add a greeting'

function git(dir: string, ...args: string[]) {
  execFileSync('git', ['-c', 'user.name=E2E', '-c', 'user.email=e2e@example.com', '-c', 'commit.gpgSign=false', ...args], { cwd: dir }) // NOSONAR: the machine's own git from PATH
}

function newRepo(prefix: string): string {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), prefix)))
  git(repo, 'init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), 'base\n')
  git(repo, 'add', '-A')
  git(repo, 'commit', '-q', '-m', 'init')
  return repo
}

const caseButton = (page: Page) => page.getByRole('button', { name: 'Talk to CASE' })
const bubble = (page: Page) => page.locator('.companion-bubble')

// Opens the bubble if it is not already open — a new approval wait
// (lib/companion.ts companionShouldOpen) can open it by itself before the
// click runs, and the button is a plain toggle, so an unconditional click
// in that case would close it right back.
async function ensureBubbleOpen(page: Page): Promise<void> {
  if (await bubble(page).isVisible()) return
  await caseButton(page).click()
  await expect(bubble(page)).toBeVisible()
}

async function autoMode(page: Page, id: string) {
  expect((await page.request.put(`/v1/admin/sessions/${encodeURIComponent(id)}/permission-mode`, { data: { mode: 'auto' } })).ok()).toBe(true)
}

type Created = { session_id: string }

// Creates a focus pipeline through the API (as the console's own "New
// task" form does: it never sets `start`) and opens its pipeline screen,
// where the store sends the goal as the first turn and the G1 plan gate
// comes back.
async function openPipelineWithGate(page: Page, goal: string, repoPrefix: string, extra: Record<string, unknown> = {}): Promise<string> {
  const repo = newRepo(repoPrefix)
  const created = (await (await page.request.post('/v1/focus/pipelines', { data: { goal, cwd: repo, ...extra } })).json()) as Created
  const id = created.session_id
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 15_000 })
  return id
}

test('CASE shows on the focus home and pipeline screens, with the sidebar still hidden (tars#1247)', async ({ page }) => {
  await page.goto('/console/focus')
  await expect(page.getByTestId('focus-home')).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await expect(caseButton(page)).toBeVisible()

  const id = await openPipelineWithGate(page, planGoal, 'tars-e2e-companion-focus-visible-')
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await expect(caseButton(page)).toBeVisible()
  void id
})

test('opening CASE\'s bubble by hand does not cover or intercept the G1 gate\'s approve button', async ({ page }) => {
  const id = await openPipelineWithGate(page, planGoal, 'tars-e2e-companion-focus-manual-')

  await caseButton(page).click()
  await expect(bubble(page)).toBeVisible()

  // The approve button is still clickable through to the real element —
  // not covered by the bubble, which would otherwise intercept the click
  // (Playwright's actionability check fails before this line if it is).
  await page.getByTestId('focus-plan-stage-pr_review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()

  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active', { timeout: 15_000 })
  void id
})

test('a bubble that opens itself from a server line while a gate is open does not intercept the gate\'s approve button either', async ({ page }) => {
  // The SSE connection has to be live before posting the e2e companion
  // event, or the page never sees it.
  const streamResponse = page.waitForResponse((res) => res.url().includes('/v1/events/stream'), { timeout: 10_000 })
  const repo = newRepo('tars-e2e-companion-focus-selfopen-')
  const created = (await (await page.request.post('/v1/focus/pipelines', { data: { goal: planGoal, cwd: repo } })).json()) as Created
  const id = created.session_id
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await streamResponse
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 15_000 })

  // A companion event about some other session opens CASE's bubble on its
  // own (lib/companion.ts companionShouldOpen) — same server-cue channel
  // companion-events.spec.ts drives.
  const res = await page.request.post('/v1/e2e/events', {
    data: {
      type: 'notification',
      category: 'companion',
      severity: 'info',
      title: '',
      message: 'something elsewhere needs you',
      session_id: 'some-other-session',
      timestamp: new Date().toISOString(),
      expression: 'alert',
    },
  })
  expect(res.ok()).toBeTruthy()
  await expect(bubble(page)).toBeVisible()

  await page.getByTestId('focus-plan-stage-pr_review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()

  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active', { timeout: 15_000 })
})

test('the pipeline screen leaves its own session\'s pending/running lines out of CASE; another session\'s still show (tars#1247)', async ({ page }) => {
  const id = await openPipelineWithGate(page, planGoal, 'tars-e2e-companion-focus-exclude-')

  const sessionsRes = await page.request.get('/v1/admin/sessions')
  const sessionsBody = await sessionsRes.json()
  const sessions = Array.isArray(sessionsBody) ? sessionsBody : sessionsBody.sessions ?? []
  const otherId = (sessions.find((s: { id: string; kind?: string }) => s.kind === 'main') ?? sessions.find((s: { id: string }) => s.id !== id))?.id
  expect(otherId).toBeTruthy()

  // Faking GET /v1/chat/activity (what sessionActivity.svelte.ts polls)
  // rather than driving a real stuck turn: deterministic, and does not
  // leave a chat slot held for the specs after this one. Both sessions get
  // a running entry and a pending approval — the real server's own
  // activityMap defaults a session named only in pending_approvals to
  // running too (a tool call pending means the turn is still active), so
  // this is the realistic shape, not an extra assumption.
  await page.route('**/v1/chat/activity', async (route) => {
    const response = await route.fetch()
    const body = await response.json()
    body.running = [
      ...(body.running ?? []),
      { session_id: id, session_title: 'Pipeline session', started_at: new Date().toISOString() },
      { session_id: otherId, session_title: 'Other session', started_at: new Date().toISOString() },
    ]
    body.pending_approvals = [
      ...(body.pending_approvals ?? []),
      { request_id: 'e2e-fake-pipeline', session_id: id, session_title: 'Pipeline session', tool_name: 'write_file' },
      { request_id: 'e2e-fake-other', session_id: otherId, session_title: 'Other session', tool_name: 'write_file' },
    ]
    await route.fulfill({ response, json: body })
  })

  const activityResponse = page.waitForResponse((res) => res.url().includes('/v1/chat/activity'))
  await page.reload()
  await activityResponse
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 15_000 })

  // Only the other session's pending approval counts toward the badge:
  // the pipeline's own pending and running are both already shown by this
  // very screen. The badge shows with the bubble closed; the lines
  // themselves are only in the DOM once it is open — and a new wait can
  // open it by itself (ensureBubbleOpen), so this must not assume it is
  // still closed.
  await expect(page.locator('.companion-pet .companion-badge')).toHaveText('1')
  await ensureBubbleOpen(page)

  const pendingLines = page.locator('.companion-line-pending')
  await expect(pendingLines).toHaveCount(1)
  await expect(pendingLines).toContainText('Other session')
  await expect(pendingLines).not.toContainText('Pipeline session')

  const runningLines = page.locator('.companion-line-running')
  await expect(runningLines).toHaveCount(1)
  await expect(runningLines).toContainText('Other session')
  await expect(runningLines).not.toContainText('Pipeline session')
})

// --- Overlap (#1194 class, requirement 3): CASE's button and bubble must
// never cover a focus screen's own buttons, at a width wide enough that
// the gutter formula protects the whole bubble (App.svelte's
// .focus-companion-space: zero padding at and above ~1552px, scaling up
// to the full bubble share by ~1216px — 1440px sits in between). Below
// ~1216px the column no longer pays that cost: only the button's own
// footprint is reserved, and a row whose own button sits at the column's
// right edge (the instruction Send button, the Q&A Ask button) can be
// covered by an open bubble there — accepted (DESIGN.md's Focus "Chrome"
// item), proven below rather than left to assumption. A left-aligned
// row (gate/finding decisions, Add finding, New task Start) stays clear
// at any width: its buttons never reach the column's own right edge.

function rectsOverlap(a: { x: number; y: number; width: number; height: number }, b: typeof a): boolean {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
}

async function noOverlapWithCase(page: Page, target: Locator): Promise<boolean> {
  // CASE is `position: fixed` (viewport-relative, unaffected by scroll);
  // a target further down the page is not, so comparing boundingBox()es
  // while still scrolled to the top (where a tall card's own buttons sit
  // below the fold at a narrow viewport height) would compare a visible
  // box against one that is not actually on screen yet. Scrolling the
  // target into view first makes the comparison answer the real
  // question: can this button ever land under CASE, at some scroll
  // position a user would actually reach to use it.
  await target.scrollIntoViewIfNeeded()
  const targetBox = await target.boundingBox()
  const petBox = await page.locator('.companion-pet').boundingBox()
  if (!targetBox || !petBox) return false
  if (rectsOverlap(targetBox, petBox)) return false
  if (await bubble(page).count()) {
    const bubbleBox = await bubble(page).boundingBox()
    if (bubbleBox && rectsOverlap(targetBox, bubbleBox)) return false
  }
  return true
}

async function expectNoOverlapWithCase(page: Page, target: Locator, label: string) {
  expect(await noOverlapWithCase(page, target), `${label} overlaps CASE`).toBe(true)
}

const usualViewport = { width: 1440, height: 900 }
const narrowViewport = { width: 1024, height: 700 }
const viewports = [
  { ...usualViewport, label: 'usual' },
  { ...narrowViewport, label: 'narrow' },
]

test('a wide enough window (>= ~1552px) keeps the focus column exactly centered — the gutter formula asks for zero padding there', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 })
  await page.goto('/console/focus')
  await caseButton(page).click()
  await expect(bubble(page)).toBeVisible()

  const screen = page.locator('.focus-home')
  const screenBox = await screen.boundingBox()
  expect(screenBox).toBeTruthy()
  // Centered within the viewport to within a pixel or two of rounding —
  // not just "clear of CASE", which a column pulled left of center would
  // also satisfy.
  const leftGap = screenBox!.x
  const rightGap = 1600 - (screenBox!.x + screenBox!.width)
  expect(Math.abs(leftGap - rightGap)).toBeLessThanOrEqual(2)
  await expectNoOverlapWithCase(page, page.getByTestId('focus-new-task-open'), 'New task (1600px)')
})

for (const viewport of viewports) {
  test(`CASE never covers the focus home, new-task or templates buttons at the ${viewport.label} size (tars#1247)`, async ({ page }) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height })

    await page.goto('/console/focus')
    await caseButton(page).click()
    await expect(bubble(page)).toBeVisible()
    await expectNoOverlapWithCase(page, page.getByTestId('focus-new-task-open'), 'New task')
    await expectNoOverlapWithCase(page, page.getByTestId('focus-release-open'), 'Release train')
    await expectNoOverlapWithCase(page, page.getByTestId('focus-templates-open'), 'Templates nav')

    await page.getByTestId('focus-new-task-open').click()
    // Left-aligned (.actions, no justify-content) — stays clear at both sizes.
    await expectNoOverlapWithCase(page, page.getByTestId('focus-new-start'), 'New task Start')

    await page.goto('/console/focus/templates')
    await expect(page.getByTestId('focus-templates')).toBeVisible()
    await expectNoOverlapWithCase(page, page.getByTestId('focus-templates-new'), 'New template')

    const id = await openPipelineWithGate(page, planGoal, `tars-e2e-companion-focus-overlap-${viewport.label}-`)
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await caseButton(page).click()
    await expect(bubble(page)).toBeVisible()
    // Left-aligned — the gate's own row never reaches the column's right
    // edge, so these stay clear at the narrow size too, not just the usual one.
    await expectNoOverlapWithCase(page, page.getByTestId('focus-gate-approve'), 'Gate approve')
    await expectNoOverlapWithCase(page, page.getByTestId('focus-qa-toggle'), 'Q&A toggle')

    const send = page.getByRole('button', { name: 'Send', exact: true })
    await page.getByTestId('focus-qa-toggle').click()
    const ask = page.getByTestId('focus-qa-ask')
    if (viewport.label === 'usual') {
      // Inside the protected band (1216px–1552px): the column's own right
      // edge sits at or inside the reserved bubble share, so even a
      // right-aligned row's own button stays clear.
      await expectNoOverlapWithCase(page, send, 'Instruction Send (usual)')
      await expectNoOverlapWithCase(page, ask, 'Q&A ask (usual)')
    } else {
      // Below the ~1216px band: only the button's own footprint is
      // reserved, so a right-aligned row's own button can land under an
      // open bubble — accepted (closing the bubble clears it). Asserted
      // as actually happening, not just left untested, so a future
      // layout change that quietly protects (or stops protecting) these
      // is caught either way.
      expect(await noOverlapWithCase(page, send), 'Instruction Send should be covered at the narrow size (accepted tradeoff)').toBe(false)
      expect(await noOverlapWithCase(page, ask), 'Q&A ask should be covered at the narrow size (accepted tradeoff)').toBe(false)
    }
    void id
  })
}

test('CASE never covers the review loop\'s finding decision or add-finding buttons, at the narrow size too', async ({ page }) => {
  const repo = newRepo('tars-e2e-companion-focus-findings-')
  const created = (await (await page.request.post('/v1/focus/pipelines', { data: { goal: reviewGoal, cwd: repo, e2e: true } })).json()) as Created
  const id = created.session_id
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 15_000 })
  writeFileSync(join(repo, 'base.txt'), 'base\nhello\n')
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()
  await expect.poll(async () => {
    const p = await (await page.request.get(`/v1/focus/pipelines/${encodeURIComponent(id)}`)).json()
    return p.open_gate ?? ''
  }, { timeout: 20_000 }).toBe('triage')

  const finding = page.locator('[data-testid="focus-card"][data-kind="finding"]')
  await expect(finding).toBeVisible()

  // Left-aligned decision/add-finding rows: proven clear at the narrow
  // size (where the gutter no longer protects a right-aligned row),
  // since a left-aligned one was never relying on that protection.
  await page.setViewportSize({ width: narrowViewport.width, height: narrowViewport.height })
  await caseButton(page).click()
  await expect(bubble(page)).toBeVisible()
  await expectNoOverlapWithCase(page, finding.getByTestId('focus-finding-fix'), 'Finding fix (narrow)')
  await expectNoOverlapWithCase(page, finding.getByTestId('focus-finding-dismiss'), 'Finding dismiss (narrow)')
  await expectNoOverlapWithCase(page, page.getByTestId('focus-add-finding'), 'Add finding (narrow)')
})

// --- Screenshots (requirement, reported to the developer — not committed):
// the focus home and a pipeline screen with its gate open, bubble open, at
// 1440x900 and 1024x700.

for (const viewport of viewports) {
  test(`screenshot: focus home and an open-gate pipeline with CASE's bubble open, at the ${viewport.label} size`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height })

    await page.goto('/console/focus')
    await caseButton(page).click()
    await expect(bubble(page)).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath(`focus-home-${viewport.label}.png`) })

    await openPipelineWithGate(page, planGoal, `tars-e2e-companion-focus-shot-${viewport.label}-`)
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await caseButton(page).click()
    await expect(bubble(page)).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath(`focus-pipeline-gate-${viewport.label}.png`) })
  })
}
