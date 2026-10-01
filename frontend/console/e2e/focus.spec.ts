// Focus mode (docs/decisions/focus-mode.md, P1b): a task started from the
// focus home runs as a pipeline on an ordinary chat session. The mock LLM
// answers the plan stage with a <focus-plan> block ([e2e:focus-plan]) and
// later stages with a <focus-report> asking one decision
// ([e2e:focus-report]); the markers ride in the goal, which every turn's
// stage guidance repeats.

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
  stages: { id: string; status: string }[]
  cards: { id: string; kind: string; state: string; decision?: string }[]
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
  await page.getByTestId('focus-new-start').click()

  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  const id = sessionId(page)

  // The goal went out as the first turn; the plan comes back as the G1 gate.
  const gate = page.locator('[data-testid="focus-card"][data-kind="gate"]')
  await expect(gate).toBeVisible()
  await expect(gate.getByText('Add greet()')).toBeVisible()
  await expect(page.getByTestId('focus-step-plan')).toHaveAttribute('data-status', 'active')

  // Skip PR review and add a verification command, then approve.
  await page.getByTestId('focus-plan-stage-pr_review').uncheck()
  await page.getByTestId('focus-plan-verify').fill('make test\ngo vet ./...')
  await page.getByTestId('focus-gate-approve').click()

  // The approval's next prompt runs the build turn: a report and a decision.
  await expect(page.getByTestId('focus-step-build')).toHaveAttribute('data-status', 'active')
  await expect(page.getByTestId('focus-step-pr_review')).toHaveAttribute('data-status', 'skipped')
  await expect(position(page)).toHaveText('1 / 2')
  await expect(card(page)).toHaveAttribute('data-kind', 'decision')
  await expect(card(page).getByText('Which greeting should greet() return?')).toBeVisible()

  // ← / → move through the deck.
  await page.keyboard.press('ArrowRight')
  await expect(position(page)).toHaveText('2 / 2')
  await expect(card(page)).toHaveAttribute('data-kind', 'report')
  await expect(card(page).getByText('Implemented greet() and its test.')).toBeVisible()
  await page.getByTestId('focus-deck-prev').click()
  await expect(card(page)).toHaveAttribute('data-kind', 'decision')

  // "View raw" shows the source turn read-only, without the hidden blocks.
  await page.getByTestId('focus-view-raw').click()
  const raw = page.getByTestId('focus-raw')
  await expect(raw.getByText('Implemented the greeting.')).toBeVisible()
  await expect(raw).not.toContainText('<focus-')

  // Key 2 picks the second option; the answer runs the next turn.
  await page.keyboard.press('2')
  await expect(page.getByText('Applied the chosen greeting.')).toBeVisible()

  const p = await pipelineOf(page, id)
  expect(p.current).toBe('build')
  expect(p.plan?.stages).toEqual(['plan', 'build', 'review', 'pr', 'merge'])
  expect(p.plan?.verify).toEqual(['make test', 'go vet ./...'])
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

  // And back.
  await page.getByTestId('focus-view-button').click()
  await expect(page).toHaveURL(new RegExp(`/console/focus/${id}$`))
  await expect(page.getByTestId('focus-pipeline')).toBeVisible()
})

test('the focus home lists the task and a stale gate action shows the current state', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-list-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal, cwd: repo } })).json()
  const id = created.session_id as string

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
})
