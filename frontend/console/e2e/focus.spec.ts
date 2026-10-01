// Focus mode (docs/decisions/focus-mode.md, P1b/P2): a task started from
// the focus home runs as a pipeline on an ordinary chat session. The mock
// LLM answers the plan stage with a <focus-plan> block ([e2e:focus-plan])
// and later stages with a <focus-report> asking one decision
// ([e2e:focus-report]), or a done report with none ([e2e:focus-loop]); the
// markers ride in the goal, which every turn's stage guidance repeats. From
// P2 the server sends every turn after the goal and runs the plan's
// verification commands after each build turn.

import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
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
  await expect(card(page).getByText('Implemented greet() and its test.')).toBeVisible()
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
  // stage drafts its PR.
  await page.keyboard.press('2')
  await expect(page.getByTestId('focus-step-pr')).toHaveAttribute('data-status', 'active', { timeout: 20_000 })
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'done')
  await expect(page.getByTestId('focus-step-review')).toHaveAttribute('data-status', 'done')
  await expect(card(page).getByText('Drafted the PR.').first()).toBeVisible()

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
  expect(p.cards.filter((c) => c.kind === 'gate')).toHaveLength(1)

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
  // review (no findings, verification passes) → pr: all on the server, the
  // console only follows.
  await expect(page.getByTestId('focus-step-pr')).toHaveAttribute('data-status', 'active', { timeout: 20_000 })
  await expect(page.getByTestId('focus-step-review')).toHaveAttribute('data-status', 'done')
  const p = await pipelineOf(page, id)
  const failure = p.cards.find((c) => c.kind === 'failure')
  expect(failure).toBeTruthy()
  expect(p.stages.find((s) => s.id === 'build')?.status).toBe('done')
  expect(p.open_gate ?? '').toBe('')

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
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: reviewGoal, cwd: repo } })).json()
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

  // The open turn runs; the probe cannot read a PR: pass by hand.
  const unavailable = page.getByTestId('focus-gh-unavailable')
  await expect(unavailable).toBeVisible({ timeout: 20_000 })
  let p = await pipelineOf(page, id)
  expect((p as Pipeline & { pr_draft?: { title: string } }).pr_draft?.title).toBe('feat: add a friendly greeting')
  expect(p.current).toBe('pr')
  await page.getByTestId('focus-gh-pass').click()

  // pr_review is skipped: merge opens G4 with its summary, no turn needed.
  await expect(page.getByTestId('focus-merge-summary')).toBeVisible()
  p = await pipelineOf(page, id)
  expect(p.current).toBe('merge')
  expect(p.open_gate).toBe('merge')
  await page.getByTestId('focus-gate-approve-generic').click()

  // The merge turn runs; gh still cannot confirm it: pass by hand.
  await expect(unavailable).toBeVisible({ timeout: 20_000 })
  await page.getByTestId('focus-gh-pass').click()
  // Not isolated: no worktree, and the banner claims no worktree outcome.
  await expect.poll(async () => ((await pipelineOf(page, id)) as Pipeline & { worktree_end?: { action: string } }).worktree_end?.action).toBe('none')
  await expect(page.getByTestId('focus-finished')).toHaveText('The pipeline is complete.')
  p = await pipelineOf(page, id)
  expect(p.stages.find((s) => s.id === 'merge')?.status).toBe('done')
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
