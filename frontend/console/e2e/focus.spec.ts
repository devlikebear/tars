// Focus mode (docs/decisions/focus-mode.md, P1b/P2): a task started from
// the focus home runs as a pipeline on an ordinary chat session. The mock
// LLM answers the plan stage with a <focus-plan> block ([e2e:focus-plan])
// and later stages with a <focus-report> asking one decision
// ([e2e:focus-report]), or a done report with none ([e2e:focus-loop]); the
// markers ride in the goal, which every turn's stage guidance repeats. From
// P2 the server sends every turn after the goal and runs the plan's
// verification commands after each build turn.

import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Locator, type Page } from '@playwright/test'

const goal = '[e2e:focus-plan] [e2e:focus-report] Add a greeting'

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

const sessionId = (page: Page) => decodeURIComponent(page.url().split('/').pop() ?? '')
const card = (page: Page) => page.getByTestId('focus-card')
const position = (page: Page) => page.getByTestId('focus-deck-position')

type Pipeline = {
  current: string
  open_gate?: string
  plan?: { stages: string[]; verify: string[] }
  stages: { id: string; status: string; iteration?: number }[]
  cards: { id: string; kind: string; state: string; decision?: string }[]
  e2e_enabled?: boolean
}

// The server sends a pipeline's turns after the goal. In a session that never
// picked a permission mode they ask before high-risk native tools through
// the ops queue (focus P2, d1); these specs opt their sessions into auto, as
// a developer who trusts the agent would, so the mock's file write runs.
async function autoMode(page: Page, id: string) {
  expect((await page.request.put(`/v1/admin/sessions/${encodeURIComponent(id)}/permission-mode`, { data: { mode: 'auto' } })).ok()).toBe(true)
}

async function pipelineOf(page: Page, id: string): Promise<Pipeline> {
  return (await page.request.get(`/v1/focus/pipelines/${encodeURIComponent(id)}`)).json()
}

test('end-to-end goals are off unless the new task turns them on', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-optin-')
  const plain = await (await page.request.post('/v1/focus/pipelines', { data: { goal: 'no goals', cwd: repo } })).json()
  expect(plain.pipeline.e2e_enabled ?? false).toBe(false)

  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  await page.getByTestId('focus-new-goal').fill(goal)
  await expect(page.getByTestId('focus-new-e2e')).not.toBeChecked()
  await expect(page.getByTestId('focus-new-e2e-hint')).toHaveCount(0)
  await page.getByTestId('focus-new-e2e').check()
  await expect(page.getByTestId('focus-new-e2e-hint')).toBeVisible()
  await page.getByTestId('focus-new-start').click()
  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  expect((await pipelineOf(page, sessionId(page))).e2e_enabled).toBe(true)
})

test('a focus task: plan gate, approve, report and decision cards, decide, then Advanced shows the same session', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-')
  await page.goto('/console/focus')
  await expect(page.getByTestId('focus-home')).toBeVisible()
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  await page.getByTestId('focus-new-goal').fill(goal)
  await page.getByTestId('focus-new-isolate').check()
  await page.getByTestId('focus-new-start').click()

  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  const id = sessionId(page)
  await autoMode(page, id)

  // Focus mode hides the app sidebar and the companion; an isolated task
  // shows its own branch.
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Talk to TARS companion' })).toHaveCount(0)
  await expect(page.getByTestId('focus-worktree-chip')).toContainText(`tars/session-${id}`)

  // At a narrow width the path shrinks to one line and the branch chip and
  // the header buttons stay whole and on screen.
  await page.setViewportSize({ width: 700, height: 900 })
  const chip = page.getByTestId('focus-worktree-chip')
  const chipBox = await chip.boundingBox()
  expect(chipBox && chipBox.x + chipBox.width).toBeLessThanOrEqual(700)
  expect(await chip.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
  const cwdLine = page.getByTestId('focus-cwd')
  await expect(cwdLine).toHaveAttribute('title', repo)
  const cwdBox = await cwdLine.boundingBox()
  expect(cwdBox?.height ?? 0).toBeLessThan(24)
  for (const control of [page.getByTestId('focus-view-advanced'), page.getByTestId('focus-open-board')]) {
    const box = await control.boundingBox()
    expect(box && box.x + box.width).toBeLessThanOrEqual(700)
  }
  await page.setViewportSize({ width: 1400, height: 900 })

  // The goal went out as the first turn; the plan comes back as the G1 gate.
  const gate = page.locator('[data-testid="focus-card"][data-kind="gate"]')
  await expect(gate).toBeVisible()
  await expect(gate.getByText('Add greet()')).toBeVisible()
  await expect(page.getByTestId('focus-step-plan')).toHaveAttribute('data-status', 'active')

  // Skip PR review and set the verification commands (they run on the
  // server after each build turn), then approve.
  await page.getByTestId('focus-plan-stage-pr_review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('true\ngit status --short')
  await page.getByTestId('focus-gate-approve').click()

  // The server runs the build turn the approval asks for: a report and a
  // decision, which pauses the loop.
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active')
  await expect(page.getByTestId('focus-step-pr_review')).toHaveAttribute('data-status', 'skipped')
  // Decision, report, and the change card of the file the turn wrote.
  await expect(position(page)).toHaveText('1 / 3')
  await expect(card(page)).toHaveAttribute('data-kind', 'decision')
  await expect(card(page).getByText('Which greeting should greet() return?')).toBeVisible()

  // ← / → move through the deck.
  await page.keyboard.press('ArrowRight')
  await expect(position(page)).toHaveText('2 / 3')
  await expect(card(page)).toHaveAttribute('data-kind', 'report')
  // A two-line summary: the heading is the first line and the body only the rest (#1109).
  await expect(card(page).locator('h3')).toHaveText('Implemented greet() and its test.')
  await expect(card(page).locator('h3')).toHaveAttribute('title', 'Implemented greet() and its test.')
  await expect(card(page).locator('p.summary')).toHaveText('The test covers both punctuation marks.')
  await expect(card(page).getByText('Implemented greet() and its test.')).toHaveCount(1)
  // A card title reads as a sentence, not a caps label.
  await expect(card(page).locator('h3')).toHaveCSS('text-transform', 'none')
  await page.keyboard.press('ArrowRight')
  await expect(card(page)).toHaveAttribute('data-kind', 'change')
  await expect(page.getByTestId('focus-change')).toContainText('greet.ts')
  await page.getByTestId('focus-deck-prev').click()
  await page.getByTestId('focus-deck-prev').click()
  await expect(card(page)).toHaveAttribute('data-kind', 'decision')

  // "View raw" shows the source turn read-only, without the hidden blocks.
  await page.getByTestId('focus-view-raw').click()
  const raw = page.getByTestId('focus-raw')
  await expect(raw.getByText('Implemented the greeting.')).toBeVisible()
  await expect(raw).toContainText('greet.ts')
  await expect(raw).not.toContainText('<focus-')
  // The raw turn — a long path, a long code line — never widens the page:
  // the deck's and the input's buttons stay on screen.
  const viewport = page.viewportSize()?.width ?? 0
  for (const control of [page.getByTestId('focus-view-raw'), page.getByRole('button', { name: 'Send' })]) {
    const box = await control.boundingBox()
    expect(box && box.x + box.width).toBeLessThanOrEqual(viewport)
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)
  expect(await page.locator('main').evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)

  // Key 2 picks the second option; the server runs the answer turn. Its
  // report says every task is done, verification passes, and review starts;
  // the review finds nothing, verification passes again (P3), and the PR
  // stage drafts its PR, which opens the G3 gate with the draft editable (P4).
  await page.keyboard.press('2')
  await expect(page.getByTestId('focus-step-pr')).toHaveAttribute('data-status', 'active', { timeout: 20_000 })
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'done')
  await expect(page.getByTestId('focus-step-review')).toHaveAttribute('data-status', 'done')
  await expect(page.getByTestId('focus-pr-gate')).toBeVisible()
  await expect(page.getByTestId('focus-pr-title')).toHaveValue('Add a greeting')

  // The build history: the newest report leads (U2), marking it seen does
  // not move the deck under the developer, and each arrow press is one card.
  await page.getByTestId('focus-step-build').click()
  await expect(position(page)).toHaveText('1 / 4')
  await expect(card(page).getByText('Applied the chosen greeting.')).toBeVisible()
  await page.keyboard.press('ArrowRight')
  await expect(position(page)).toHaveText('2 / 4')
  await page.keyboard.press('ArrowRight')
  await expect(position(page)).toHaveText('3 / 4')
  await page.keyboard.press('ArrowLeft')
  await expect(position(page)).toHaveText('2 / 4')

  await page.getByRole('button', { name: 'Back to the current stage' }).click()

  const p = await pipelineOf(page, id)
  expect(p.current).toBe('pr')
  expect(p.plan?.stages).toEqual(['plan', 'build', 'review', 'pr', 'merge'])
  expect(p.plan?.verify).toEqual(['true', 'git status --short'])
  expect(p.cards.find((c) => c.kind === 'decision')).toMatchObject({ state: 'decided', decision: 'Hi there' })
  // G1 (decided) and G3 (open: the draft waits for the developer).
  expect(p.cards.filter((c) => c.kind === 'gate')).toHaveLength(2)
  expect(p.open_gate).toBe('pr')

  // A reload rebuilds the same deck: no duplicate cards.
  const total = (text: string | null) => text?.split('/')[1]?.trim()
  const before = total(await position(page).textContent())
  await page.reload()
  await expect(position(page)).toHaveText(new RegExp(`^\\d+ / ${before}$`))
  expect((await pipelineOf(page, id)).cards).toHaveLength(p.cards.length)

  // The same session in Advanced, with the hidden blocks folded away.
  await page.getByTestId('focus-view-advanced').click()
  await expect(page).toHaveURL(new RegExp(`/console/chat/${id}$`))
  const thread = page.locator('.chat-main')
  await expect(thread.getByText('Implemented the greeting.')).toBeVisible()
  await expect(thread.locator('.chat-user').first()).toContainText('Add a greeting')
  await expect(thread).not.toContainText('<focus-')
  await expect(thread).not.toContainText('current stage:')
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()

  // And back.
  await page.getByTestId('focus-view-button').click()
  await expect(page).toHaveURL(new RegExp(`/console/focus/${id}$`))
  await expect(page.getByTestId('focus-pipeline')).toBeVisible()
})

// A task can reach more than one folder: the primary stays the only
// active/isolated one, but the extras become registered work dirs the
// session can read and write through its tools (#1147 et al.).
test('a new task with an extra folder registers both as the session\'s work dirs', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-multi-primary-')
  const extra = newRepo('tars-e2e-focus-multi-extra-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  await page.getByTestId('focus-new-extra-add').click()
  await page.getByTestId('focus-new-extra-folder').fill(extra)
  await expect(page.getByTestId('focus-new-extra-status')).toHaveText('Git repository')

  await page.getByTestId('focus-new-goal').fill(goal)
  await page.getByTestId('focus-new-start').click()

  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  const id = sessionId(page)
  const sess = await (await page.request.get(`/v1/admin/sessions/${encodeURIComponent(id)}`)).json()
  const workDirs = (sess.work_dirs as string[]) ?? []
  expect(workDirs.some((d) => d === repo)).toBe(true)
  expect(workDirs.some((d) => d === extra)).toBe(true)
  // The primary alone is the active cwd.
  expect(sess.current_dir).toBe(repo)
})

test('the focus home lists the task and a stale gate action shows the current state', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-list-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal, cwd: repo } })).json()
  const id = created.session_id as string
  await autoMode(page, id)

  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()

  // Another tab approves first; this one's approve meets a 409.
  const other = await page.request.post(`/v1/focus/pipelines/${id}/gates/plan`, { data: { action: 'approve' } })
  expect(other.ok()).toBeTruthy()
  await page.getByTestId('focus-gate-approve').click()
  await expect(page.getByText('The pipeline changed elsewhere; this view now shows its current state.')).toBeVisible()
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active')

  await page.goto('/console/focus')
  await expect(page.getByTestId('focus-task').filter({ hasText: 'Add a greeting' }).first()).toBeVisible()
})

test('plan gate edits survive a question turn that re-reads the pipeline', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-edit-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal, cwd: repo } })).json()
  const id = created.session_id as string
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()

  await page.getByTestId('focus-plan-stage-review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('make test\nmake lint')
  // A question while the gate is open: the turn ends and the pipeline is read again.
  await page.getByTestId('focus-instruction').fill('[e2e:focus-ask] are these commands enough?')
  await page.getByTestId('focus-instruction').press('Enter')
  await expect(page.getByTestId('focus-progress')).toBeVisible()
  await expect(page.getByTestId('focus-progress')).toBeHidden()

  await expect(page.getByTestId('focus-plan-verify')).toHaveValue('make test\nmake lint')
  await expect(page.getByTestId('focus-plan-stage-review')).not.toBeChecked()
})

test('the build loop: a failed verification becomes a failure card and a fix turn, a pass moves to review, and Q&A answers about a card', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-loop-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: '[e2e:focus-plan] [e2e:focus-loop] Add a greeting', cwd: repo } })).json()
  const id = created.session_id as string
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()

  // A verification command that fails the first time and passes after.
  const verify = `sh -c 'test -f .e2e-verified || { touch .e2e-verified; echo "--- FAIL: TestGreet" >&2; exit 1; }'`
  await page.getByTestId('focus-plan-verify').fill(verify)
  await page.getByTestId('focus-gate-approve').click()

  // Build turn → verification fails → failure card + fix turn → passes →
  // review (no findings, verification passes) → pr, whose draft opens G3:
  // all on the server, the console only follows. The pipeline rests at G3
  // (P4) — a fact that does not depend on timing, unlike "pr is active and
  // no gate is open yet", which raced the draft turn on fast runners.
  await expect(page.getByTestId('focus-pr-gate')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByTestId('focus-step-review')).toHaveAttribute('data-status', 'done')
  const p = await pipelineOf(page, id)
  const failure = p.cards.find((c) => c.kind === 'failure')
  expect(failure).toBeTruthy()
  expect(p.stages.find((s) => s.id === 'build')?.status).toBe('done')
  expect(p.current).toBe('pr')
  expect(p.open_gate).toBe('pr')

  // The failure card sits in the build history with its command and output.
  await page.getByTestId('focus-step-build').click()
  const failureCard = page.locator('[data-testid="focus-card"][data-kind="failure"]')
  for (let i = 0; i < 6 && !(await failureCard.isVisible()); i++) await page.keyboard.press('ArrowRight')
  await expect(failureCard).toBeVisible()
  await expect(failureCard.getByTestId('focus-failure-excerpt')).toContainText('--- FAIL: TestGreet')

  // `?` opens the card's Q&A drawer; the answer threads under the card.
  await page.keyboard.press('?')
  const qaInput = page.getByTestId('focus-qa-input')
  await expect(qaInput).toBeFocused()
  await qaInput.fill('why did it fail?')
  await qaInput.press('Enter')
  const answer = page.getByTestId('focus-qa-answer')
  await expect(answer).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('focus-qa-entry')).toHaveCount(1)
  // The answer came from the hidden Q&A session, not the pipeline's.
  const after = await (await page.request.get(`/v1/focus/pipelines/${encodeURIComponent(id)}`)).json()
  expect(after.qa_session_id).toBeTruthy()
  expect(after.qa_turns[failure!.id]).toEqual([1])
  expect(after.cards).toHaveLength((await pipelineOf(page, id)).cards.length)

  // Promote to instruction fills the stage input as a draft; nothing is sent.
  await page.getByTestId('focus-qa-promote').click()
  await expect(page.getByTestId('focus-instruction')).toHaveValue(/^About "Verification failed: sh -c/)
  expect((await pipelineOf(page, id)).current).toBe('pr')
})

// The review loop (P3): two findings open the triage gate and arrive one card
// at a time; fixing one and dismissing the other sends one fix turn, after
// which verification (verify and end-to-end commands) runs and a second
// review round finds nothing, so review is done.
const reviewGoal = '[e2e:focus-plan] [e2e:focus-review] Add a greeting'

// startReview creates a review-loop pipeline and opens it (the console sends
// the first turn), changes base.txt after its base commit, and approves the plan with a passing
// verification command; it returns the session id once triage is open.
async function startReview(page: Page, approveInUI: boolean): Promise<string> {
  const repo = newRepo('tars-e2e-focus-review-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: reviewGoal, cwd: repo, e2e: true } })).json()
  const id = created.session_id as string
  await autoMode(page, id)
  // The console sends the goal as the first turn; it records the base commit.
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 15_000 })
  writeFileSync(join(repo, 'base.txt'), 'base\nhello\n')
  if (approveInUI) {
    await page.getByTestId('focus-plan-verify').fill('true')
    await page.getByTestId('focus-gate-approve').click()
  } else {
    const plan = (await pipelineOf(page, id)).plan!
    const res = await page.request.post(`/v1/focus/pipelines/${encodeURIComponent(id)}/gates/plan`, { data: { action: 'approve', edits: { ...plan, verify: ['true'] } } })
    expect(res.ok()).toBe(true)
  }
  await expect.poll(async () => (await pipelineOf(page, id)).open_gate ?? '', { timeout: 20_000 }).toBe('triage')
  return id
}

test('the review loop: findings are triaged one at a time, a fix turn and verification run, and a clean round ends review', async ({ page }) => {
  const id = await startReview(page, true)
  const finding = page.locator('[data-testid="focus-card"][data-kind="finding"]')
  const progress = page.getByTestId('focus-triage-progress')

  // The first finding: severity, location, scenario and the diff around it.
  await expect(finding).toBeVisible()
  await expect(progress).toHaveText('0 / 2 decided')
  await expect(finding.getByTestId('focus-finding-severity')).toHaveText('high')
  await expect(finding.getByTestId('focus-finding-loc')).toHaveText('base.txt:2')
  await expect(finding.getByText('the UI shows a bare word')).toBeVisible()
  await expect(finding.getByTestId('focus-finding-excerpt')).toContainText('+hello')

  // Ask opens the card's Q&A drawer, like `?`.
  await finding.getByTestId('focus-finding-ask').click()
  await expect(page.getByTestId('focus-qa-input')).toBeFocused()

  // One at a time: Fix hands over to the second finding; triage stays open.
  await finding.getByTestId('focus-finding-fix').click()
  await expect(progress).toHaveText('1 / 2 decided')
  await expect(finding.getByTestId('focus-finding-loc')).toHaveText('base.txt:1')
  expect((await pipelineOf(page, id)).open_gate).toBe('triage')

  // The last decision closes triage; the server sends the fix turn with the
  // accepted finding only, verifies, reviews again, and moves on.
  await finding.getByTestId('focus-finding-dismiss').click()
  await expect(page.getByTestId('focus-step-review')).toHaveAttribute('data-status', 'done', { timeout: 30_000 })
  await expect(progress).toHaveCount(0)

  const p = await pipelineOf(page, id)
  const findings = p.cards.filter((c) => c.kind === 'finding')
  expect(findings.map((c) => c.decision)).toEqual(['fix', 'dismiss'])
  expect(p.stages.find((s) => s.id === 'review')?.iteration).toBe(2)
  expect(p.current).toBe('pr')

  // The plan's end-to-end goal ran, on the e2e cua-driver stub (CUA_DRIVER_PATH)
  // rather than being skipped or reaching a driver installed on the host.
  const driverCalls = readFileSync(join(process.env.TARS_E2E_WORKSPACE!, 'e2e-cua-driver.log'), 'utf8').trim().split('\n')
  expect(driverCalls).toContain('get_window_state')
  expect(driverCalls.every((tool) => tool === 'list_windows' || tool === 'get_window_state')).toBe(true)

  // The fix turn named the accepted finding only.
  const history = await (await page.request.get(`/v1/admin/sessions/${encodeURIComponent(id)}/history`)).json() as { role: string; content: string }[]
  const fix = history.find((m) => m.role === 'user' && m.content.startsWith('Fix these findings'))
  expect(fix?.content).toContain('base.txt:2')
  expect(fix?.content).not.toContain('base.txt:1')

  // A stale tab deciding again gets 409 and the current pipeline.
  const again = await page.request.post(`/v1/focus/pipelines/${encodeURIComponent(id)}/cards/${findings[0].id}`, { data: { state: 'decided', decision: 'dismiss' } })
  expect(again.status()).toBe(409)
})

test('the PR stages: G3 opens the PR with an edited title, gh unavailable is passed by hand, G4 merges, and the pipeline finishes', async ({ page }) => {
  // No GitHub remote: the server's gh probe cannot read a PR (gh missing,
  // logged out, or no remote all come back unavailable), which is the path
  // a developer without gh takes.
  const repo = newRepo('tars-e2e-focus-pr-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: '[e2e:focus-plan] [e2e:focus-loop] [e2e:focus-pr] Add a greeting', cwd: repo } })).json()
  const id = created.session_id as string
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()
  await page.getByTestId('focus-plan-stage-review').uncheck()
  await page.getByTestId('focus-plan-stage-pr_review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()

  // Build passes; the pr stage's draft opens G3, editable.
  const prGate = page.getByTestId('focus-pr-gate')
  await expect(prGate).toBeVisible({ timeout: 20_000 })
  await expect(page.getByTestId('focus-pr-title')).toHaveValue('feat: add a greeting')
  await page.getByTestId('focus-pr-title').fill('feat: add a friendly greeting')
  await page.getByTestId('focus-pr-approve').click()

  // The open turn runs; then the probe cannot read a PR. Await that exact
  // server state — the stub's error recorded and no turn owed (#1082: a
  // probe never runs while the write turn is owed) — not a banner timing.
  type PRPipeline = Pipeline & { pr_draft?: { title: string }; pr_unavailable?: string; pending_turn?: string; worktree_end?: { action: string } }
  const unavailableNow = async () => {
    const q = (await pipelineOf(page, id)) as PRPipeline
    return !q.pending_turn && (q.pr_unavailable ?? '').includes('e2e gh stub')
  }
  // passByHand clicks the banner's pass and requires the server to take it.
  const passByHand = async () => {
    await expect(page.getByTestId('focus-gh-unavailable')).toBeVisible()
    const advanced = page.waitForResponse((r) => r.url().endsWith(`/v1/focus/pipelines/${id}/advance`) && r.request().method() === 'POST')
    await page.getByTestId('focus-gh-pass').click()
    expect((await advanced).status()).toBe(200)
  }
  await expect.poll(unavailableNow, { timeout: 20_000 }).toBe(true)
  let p = (await pipelineOf(page, id)) as PRPipeline
  expect(p.pr_draft?.title).toBe('feat: add a friendly greeting')
  // The e2e gh stub answered, not a host gh (TARS_FOCUS_GH_PATH).
  expect(p.pr_unavailable).toContain('e2e gh stub')
  expect(p.current).toBe('pr')
  await passByHand()

  // pr_review is skipped: merge opens G4 with its summary, no turn needed.
  await expect(page.getByTestId('focus-merge-summary')).toBeVisible()
  p = (await pipelineOf(page, id)) as PRPipeline
  expect(p.current).toBe('merge')
  expect(p.open_gate).toBe('merge')
  await page.getByTestId('focus-gate-approve-generic').click()

  // The merge turn runs; then gh still cannot confirm it: pass by hand.
  await expect.poll(unavailableNow, { timeout: 20_000 }).toBe(true)
  await passByHand()
  // The pass finished the pipeline and recorded the worktree outcome in the
  // same request: not isolated, so no worktree, and the banner claims none.
  p = (await pipelineOf(page, id)) as PRPipeline
  expect(p.worktree_end?.action).toBe('none')
  await expect(page.getByTestId('focus-finished')).toHaveText('The pipeline is complete.')
  p = await pipelineOf(page, id)
  expect(p.stages.find((s) => s.id === 'merge')?.status).toBe('done')
})

test('the PR stages with gh: the PR is found, CI is green, G4 merges, and the merged pipeline finishes', async ({ page }) => {
  // The e2e gh stub answers from .git/e2e-gh in the repository: an open PR
  // #7 from the checked-out branch with one passing check and SonarCloud's
  // informational comment, then merged.
  const repo = newRepo('tars-e2e-focus-merge-')
  const scenario = (s: string) => writeFileSync(join(repo, '.git', 'e2e-gh'), s)
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: '[e2e:focus-plan] [e2e:focus-loop] [e2e:focus-pr] Add a greeting', cwd: repo } })).json()
  const id = created.session_id as string
  await autoMode(page, id)
  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()
  await page.getByTestId('focus-plan-stage-review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('true')
  await page.getByTestId('focus-gate-approve').click()

  await expect(page.getByTestId('focus-pr-gate')).toBeVisible({ timeout: 20_000 })
  scenario('open')
  await page.getByTestId('focus-pr-approve').click()

  // The open turn ends; the probe finds PR #7 green: pr_review passes and
  // G4 opens with the facts. The bot's comment is no finding (#1094).
  const summary = page.getByTestId('focus-merge-summary')
  await expect(summary).toBeVisible({ timeout: 20_000 })
  await expect(summary).toContainText('PR #7')
  await expect(page.getByTestId('focus-step-pr-pr_review')).toHaveText('#7')
  let p = await pipelineOf(page, id)
  expect(p.current).toBe('merge')
  expect(p.open_gate).toBe('merge')
  expect(p.cards.filter((c) => c.kind === 'finding')).toEqual([])

  scenario('merged')
  await page.getByTestId('focus-gate-approve-generic').click()
  // The merge turn ends; the pinned probe (gh pr view 7) sees it merged.
  await expect(page.getByTestId('focus-finished')).toHaveText('Merged.', { timeout: 20_000 })
  p = await pipelineOf(page, id)
  expect((p as Pipeline & { pr?: { state: string } }).pr?.state).toBe('MERGED')
  await expect.poll(async () => ((await pipelineOf(page, id)) as Pipeline & { worktree_end?: { action: string } }).worktree_end?.action).toBe('none')
  const calls = readFileSync(join(repo, '.git', 'e2e-gh.log'), 'utf8').trim().split('\n')
  expect(calls[0]).toMatch(/^pr view --json /)
  expect(calls.at(-1)).toMatch(/^pr view 7 --json /)
})

test('an image pasted into the new task goal field rides the first turn as an attachment (#1097)', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-image-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  const goalField = page.getByTestId('focus-new-goal')
  await goalField.fill(goal)

  // A real OS paste of a screenshot is a DataTransfer carrying an image
  // File; synthesize that rather than relying on the OS clipboard, which
  // CI has none of.
  await goalField.evaluate((el) => {
    const data = new DataTransfer()
    data.items.add(new File([new Uint8Array([1, 2, 3, 4])], 'ignored.png', { type: 'image/png' }))
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  })
  await expect(page.getByTestId('focus-new-image')).toHaveCount(1)
  // The goal text pasting an image must not touch is still there.
  await expect(goalField).toHaveValue(goal)

  const firstTurn = page.waitForRequest((req) => req.method() === 'POST' && new URL(req.url()).pathname === '/v1/chat')
  await page.getByTestId('focus-new-start').click()
  const body = (await firstTurn).postDataJSON() as { attachments?: { name: string; mime_type: string; data: string }[] }
  expect(body.attachments).toHaveLength(1)
  expect(body.attachments?.[0].mime_type).toBe('image/png')
  expect(body.attachments?.[0].data.length).toBeGreaterThan(0)

  // Removing it before starting drops it from the first turn.
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  await page.getByTestId('focus-new-goal').fill(goal)
  await page.getByTestId('focus-new-goal').evaluate((el) => {
    const data = new DataTransfer()
    data.items.add(new File([new Uint8Array([1, 2, 3, 4])], 'ignored.png', { type: 'image/png' }))
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  })
  await page.getByTestId('focus-new-image-remove').click()
  await expect(page.getByTestId('focus-new-image')).toHaveCount(0)
  const secondTurn = page.waitForRequest((req) => req.method() === 'POST' && new URL(req.url()).pathname === '/v1/chat')
  await page.getByTestId('focus-new-start').click()
  const secondBody = (await secondTurn).postDataJSON() as { attachments?: unknown[] }
  expect(secondBody.attachments ?? []).toHaveLength(0)
})

test('Browse… opens the shared folder dialog; choosing a folder in it fills the field and checks it', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-browse-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()

  // No folder checked yet: the dialog opens at the server's home folder.
  await page.getByTestId('focus-new-folder-browse').click()
  const dialog = page.getByTestId('focus-new-folder-dialog')
  await expect(dialog).toBeVisible()
  const dialogPath = dialog.getByRole('textbox', { name: 'Folder path' })
  await expect(dialogPath).not.toHaveValue('')

  await dialogPath.fill(repo)
  await dialogPath.press('Enter')
  await expect(dialogPath).toHaveValue(repo)
  await dialog.getByRole('button', { name: 'Select Here' }).click()
  await expect(dialog).toHaveCount(0)

  await expect(page.getByTestId('focus-new-folder')).toHaveValue(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  await page.getByTestId('focus-new-goal').fill(goal)
  await page.getByTestId('focus-new-start').click()
  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
})

test('Escape in the folder dialog leaves the new-task form as it was', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-browse-cancel-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  // Opens at the folder already checked, not at home.
  await page.getByTestId('focus-new-folder-browse').click()
  const dialog = page.getByTestId('focus-new-folder-dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('textbox', { name: 'Folder path' })).toHaveValue(repo)

  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(page.getByTestId('focus-new-folder')).toHaveValue(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  // The dialog's own Cancel button closes it the same way.
  await page.getByTestId('focus-new-folder-browse').click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByTestId('focus-new-folder')).toHaveValue(repo)
})

test('Escape closes the folder dialog wherever focus is, after a field has used it for itself', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-browse-escape-')
  mkdirSync(join(repo, 'sub'))
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  const browse = page.getByTestId('focus-new-folder-browse')
  const dialog = page.getByTestId('focus-new-folder-dialog')
  const dialogPath = dialog.getByRole('textbox', { name: 'Folder path' })

  // Opening a folder replaces the list under the pointer, so focus falls
  // out of the dialog — Escape still closes it.
  await browse.click()
  await dialog.getByRole('button', { name: 'sub' }).click()
  await expect(dialogPath).toHaveValue(join(repo, 'sub'))
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  // Focus is back on the button that opened it.
  await expect(browse).toBeFocused()

  // In the path field the first Escape puts the typed edit back, and only
  // the next one — with nothing left to revert — closes the dialog.
  await browse.click()
  await expect(dialogPath).toHaveValue(repo)
  await dialogPath.fill(join(repo, 'typo'))
  await dialogPath.press('Escape')
  await expect(dialog).toBeVisible()
  await expect(dialogPath).toHaveValue(repo)
  await dialogPath.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(page.getByTestId('focus-new-folder')).toHaveValue(repo)
})

test('an extra folder row has its own Browse… that fills that row', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-browse-extra-main-')
  const extra = newRepo('tars-e2e-focus-browse-extra-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')

  await page.getByTestId('focus-new-extra-add').click()
  await page.getByTestId('focus-new-extra-browse').click()
  const dialog = page.getByTestId('focus-new-folder-dialog')
  const dialogPath = dialog.getByRole('textbox', { name: 'Folder path' })
  // An empty row opens at the primary folder.
  await expect(dialogPath).toHaveValue(repo)
  await dialogPath.fill(extra)
  await dialogPath.press('Enter')
  await expect(dialogPath).toHaveValue(extra)
  await dialog.getByRole('button', { name: 'Select Here' }).click()
  await expect(dialog).toHaveCount(0)

  await expect(page.getByTestId('focus-new-extra-folder')).toHaveValue(extra)
  await expect(page.getByTestId('focus-new-extra-status')).toHaveText('Git repository')
  // The primary folder is untouched.
  await expect(page.getByTestId('focus-new-folder')).toHaveValue(repo)
})

// --- Korean (see e2e/workbench-ko.spec.ts) ---

const keptInEnglish = ['TARS', 'Git', 'PR', 'cwd', 'diff', 'Ctrl', 'Cmd', 'Enter', 'Esc']
const englishRun = /[A-Za-z]{2,}[ \t]+[A-Za-z]{2,}/

function untranslated(text: string): boolean {
  let rest = text
  for (const name of keptInEnglish) rest = rest.replaceAll(name, ' ')
  return englishRun.test(rest)
}

async function chromeTexts(scope: Locator): Promise<string[]> {
  return scope.evaluate((root) => {
    const seen = new Set<string>()
    const add = (value: string | null | undefined) => {
      const text = value?.replace(/\s+/g, ' ').trim()
      if (text) seen.add(text)
    }
    const content = '.markdown-body, pre, code, .chat-msg, [data-content]'
    const ownText = (el: Element) => {
      const copy = el.cloneNode(true) as Element
      copy.querySelectorAll(content).forEach((node) => node.remove())
      return copy.textContent
    }
    root.querySelectorAll('button, label, h1, h2, h3, h4, th, legend, summary, p, .badge').forEach((el) => {
      if (!el.closest(content)) add(ownText(el))
    })
    root.querySelectorAll('[title], [aria-label], [placeholder]').forEach((el) => {
      if (el.closest(content)) return
      add(el.getAttribute('title'))
      add(el.getAttribute('aria-label'))
      add(el.getAttribute('placeholder'))
    })
    return [...seen]
  })
}

test.describe('Korean', () => {
  test.use({ locale: 'ko-KR' })

  test('the focus home and pipeline screen chrome is Korean', async ({ page }) => {
    const repo = newRepo('tars-e2e-focus-ko-')
    await page.goto('/console/focus')
    await expect(page.getByTestId('focus-home')).toBeVisible()
    await page.getByTestId('focus-new-task-open').click()
    await page.getByTestId('focus-new-folder').fill(repo)
    await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git 저장소')
    const home = await chromeTexts(page.getByTestId('focus-home'))
    expect(home.length).toBeGreaterThan(5)
    expect(home.filter(untranslated)).toEqual([])

    await page.getByTestId('focus-new-goal').fill(goal)
    await page.getByTestId('focus-new-start').click()
    await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()
    const screen = await chromeTexts(page.getByTestId('focus-pipeline'))
    expect(screen.length).toBeGreaterThan(5)
    expect(screen.filter(untranslated)).toEqual([])
  })

  test('a finding card and triage progress are Korean', async ({ page }) => {
    const id = await startReview(page, false)
    await page.goto(`/console/focus/${id}`)
    const finding = page.locator('[data-testid="focus-card"][data-kind="finding"]')
    await expect(finding).toBeVisible()
    await expect(page.getByTestId('focus-triage-progress')).toHaveText('2개 중 0개 결정')
    await expect(finding.getByTestId('focus-finding-severity')).toHaveText('높음')
    const screen = await chromeTexts(page.getByTestId('focus-pipeline'))
    expect(screen.filter(untranslated)).toEqual([])
  })
})
