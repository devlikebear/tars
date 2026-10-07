// Focus templates and goal mode (docs/decisions/focus-mode.md §4): a task
// started from the "Writing" template runs outline → draft → revise on the
// same pipeline machinery, and in goal mode nobody clicks anything — the
// server approves the plan, triages the findings (the high one is fixed, the
// low one dismissed) and runs to the end. The mock LLM plays any template
// under [e2e:focus-template].

import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const goal = '[e2e:focus-template] A short story about a lighthouse'

function newRepo(prefix: string): string {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), prefix)))
  const git = (...args: string[]) =>
    execFileSync('git', ['-c', 'user.name=E2E', '-c', 'user.email=e2e@example.com', '-c', 'commit.gpgSign=false', ...args], { cwd: repo }) // NOSONAR: the machine's own git from PATH
  git('init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), 'base\n')
  git('add', '-A')
  git('commit', '-q', '-m', 'init')
  return repo
}

type Pipeline = {
  current: string
  template?: string
  open_gate?: string
  finished_at?: string
  goal_mode?: { enabled: boolean; pushes: number; end_reason?: string }
  stages: { id: string; kind?: string; status: string }[]
  cards: { kind: string; stage: string; state: string; decision?: string; title: string }[]
}

const sessionId = (page: Page) => decodeURIComponent(page.url().split('/').pop() ?? '')

async function pipelineOf(page: Page, id: string): Promise<Pipeline> {
  return (await page.request.get(`/v1/focus/pipelines/${encodeURIComponent(id)}`)).json()
}

test('a writing task in goal mode runs outline, draft and revise to the end with nobody at the gates', async ({ page }) => {
  const repo = newRepo('tars-e2e-focus-writing-')
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  await page.getByTestId('focus-new-folder').fill(repo)
  await expect(page.getByTestId('focus-new-folder-status')).toHaveText('Git repository')
  await page.getByTestId('focus-new-goal').fill(goal)

  // The template picker lists the built-in templates and shows the stages.
  const picker = page.getByTestId('focus-new-template')
  await expect(picker.locator('option')).toHaveText(['Development', 'Writing', 'Research'])
  await picker.selectOption('writing')
  await expect(page.getByTestId('focus-new-template-stages')).toContainText('Stages: Outline → Draft → Revise')
  await page.getByTestId('focus-new-goal-mode').check()
  await expect(page.getByTestId('focus-new-goal-hint')).toBeVisible()
  await page.getByTestId('focus-new-start').click()

  await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  const id = sessionId(page)

  // The stepper is the template's: three stages under their own names.
  await expect(page.getByTestId('focus-step-plan')).toContainText('Outline')
  await expect(page.getByTestId('focus-step-draft')).toContainText('Draft')
  await expect(page.getByTestId('focus-step-revise')).toContainText('Revise')
  await expect(page.getByTestId('focus-step-build')).toHaveCount(0)
  await expect(page.getByTestId('focus-goal-toggle')).toHaveAttribute('aria-pressed', 'true')
  // Goal mode runs the session's tools without asking.
  expect((await (await page.request.get(`/v1/admin/sessions/${encodeURIComponent(id)}/permission-mode`)).json()).mode).toBe('auto')

  // Nothing is clicked from here on.
  await expect
    .poll(async () => (await pipelineOf(page, id)).goal_mode?.end_reason ?? '', { timeout: 40_000, intervals: [500] })
    .toBe('finished')
  const p = await pipelineOf(page, id)
  expect(p.template).toBe('writing')
  expect(p.stages.map((s) => [s.id, s.kind ?? s.id, s.status])).toEqual([
    ['plan', 'plan', 'done'],
    ['draft', 'build', 'done'],
    ['revise', 'review', 'done'],
  ])
  expect(p.finished_at).toBeTruthy()
  expect(p.goal_mode?.enabled).toBe(false)
  expect(p.goal_mode?.pushes).toBe(0)
  // The plan was approved and the findings triaged by policy.
  expect(p.cards.filter((c) => c.kind === 'gate').map((c) => c.decision)).toEqual(['approve'])
  expect(p.cards.filter((c) => c.kind === 'finding').map((c) => [c.stage, c.decision])).toEqual([
    ['revise', 'fix'],
    ['revise', 'dismiss'],
  ])
  // The session has its own permission mode back.
  expect((await (await page.request.get(`/v1/admin/sessions/${encodeURIComponent(id)}/permission-mode`)).json()).mode ?? '').toBe('')

  await expect(page.getByTestId('focus-step-revise')).toHaveAttribute('data-status', 'done', { timeout: 15_000 })
  await page.goto('/console/focus')
  await expect(page.getByTestId('focus-task').first()).toContainText('Revise')
})

test('a workspace template file is offered, and goal mode can be turned on and off on a running task', async ({ page }) => {
  // The workspace's focus-templates folder: one valid template, one broken.
  const dir = join(process.env.TARS_E2E_WORKSPACE as string, 'focus-templates')
  mkdirSync(dir, { recursive: true })
  writeFileSync(join(dir, 'blog.yaml'), 'name: Blog post\ndescription: Outline, then write.\nstages:\n  - id: plan\n    label: Outline\n  - id: write\n    kind: build\n    label: Write\n')
  writeFileSync(join(dir, 'broken.yaml'), 'name: Broken\n')

  const listed = await (await page.request.get('/v1/focus/templates')).json()
  expect(listed.templates.map((t: { id: string }) => t.id)).toEqual(['dev', 'writing', 'research', 'blog'])
  expect(listed.diagnostics.map((d: { source: string }) => d.source)).toEqual(['broken.yaml'])

  const repo = newRepo('tars-e2e-focus-blog-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal, cwd: repo, template: 'blog' } })).json()
  const id = created.session_id as string
  await page.goto(`/console/focus/${id}`)
  await expect(page.getByTestId('focus-step-write')).toContainText('Write')

  // The plan gate waits for the developer; turning goal mode on approves it.
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible({ timeout: 20_000 })
  await expect(page.getByTestId('focus-plan-stage-write')).toBeChecked()
  const toggle = page.getByTestId('focus-goal-toggle')
  await expect(toggle).toHaveAttribute('aria-pressed', 'false')
  page.once('dialog', (dialog) => void dialog.accept())
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-pressed', 'true')
  await expect
    .poll(async () => (await pipelineOf(page, id)).goal_mode?.end_reason ?? '', { timeout: 40_000, intervals: [500] })
    .toBe('finished')
  expect((await pipelineOf(page, id)).stages.map((s) => s.status)).toEqual(['done', 'done'])
})

// --- AI-assisted template editing (ADR §4.1 follow-up) ---
//
// [e2e:focus-template-edit] drives frontend/console/e2e/mock-llm.mjs's
// focusTemplateDraftReply, which switches on a second word in the same
// request text — see its comment for exactly what each reply holds. The
// scenario cleans up the template it creates so later specs in this file
// (and this file's own earlier specs) see the same built-in-only baseline.
const templateEditMarker = '[e2e:focus-template-edit]'

test('AI-assisted template editing: create, appear in the New task picker, edit, and delete', async ({ page }) => {
  await page.goto('/console/focus/templates')
  await expect(page.getByTestId('focus-templates')).toBeVisible()

  // Create: the draft never writes a file until Save.
  await page.getByTestId('focus-templates-new').click()
  await page.getByTestId('focus-templates-request').fill(`${templateEditMarker} a three-stage story template: outline, draft, revise`)
  await page.getByTestId('focus-templates-draft').click()
  await expect(page.getByTestId('focus-templates-preview')).toBeVisible()
  await expect(page.getByTestId('focus-templates-preview-summary')).toContainText('three-stage story template')
  await expect(page.getByTestId('focus-templates-preview-stage')).toHaveCount(3)
  await expect(page.locator('[data-testid="focus-templates-preview-stage"][data-change="added"]')).toHaveCount(3)
  let listed = await (await page.request.get('/v1/focus/templates')).json()
  expect(listed.templates.map((t: { id: string }) => t.id)).not.toContain('lighthouse-story')

  await page.getByTestId('focus-templates-save').click()
  await expect(page.getByTestId('focus-templates-editor')).toHaveCount(0)
  const row = page.locator('[data-testid="focus-templates-row"][data-template-id="lighthouse-story"]')
  await expect(row).toBeVisible()
  await expect(row).toContainText('Lighthouse story')
  await expect(row).toContainText('custom')

  // The saved template is offered where any focus task starts.
  await page.goto('/console/focus')
  await page.getByTestId('focus-new-task-open').click()
  const picker = page.getByTestId('focus-new-template')
  await expect(picker.locator('option', { hasText: 'Lighthouse story' })).toHaveCount(1)
  await picker.selectOption('lighthouse-story')
  await expect(page.getByTestId('focus-new-template-stages')).toContainText('Stages: Outline → Draft → Revise')

  // Edit: a follow-up natural-language request adds a stage.
  await page.goto('/console/focus/templates')
  await row.getByTestId('focus-templates-edit').click()
  await expect(page.getByTestId('focus-templates-editor')).toContainText('Lighthouse story')
  await page.getByTestId('focus-templates-request').fill(`${templateEditMarker} add an illustration stage before revise`)
  await page.getByTestId('focus-templates-draft').click()
  await expect(page.getByTestId('focus-templates-preview-summary')).toContainText('Added an illustration stage')
  const addedStage = page.locator('[data-testid="focus-templates-preview-stage"][data-change="added"]')
  await expect(addedStage).toHaveCount(1)
  await expect(addedStage).toContainText('Illustrate')
  await expect(page.locator('[data-testid="focus-templates-preview-stage"][data-change="unchanged"]')).toHaveCount(3)
  await page.getByTestId('focus-templates-save').click()
  await expect(row).toContainText('Outline, draft, illustrate, revise.')

  // Delete: drafted and confirmed, same as save.
  await row.getByTestId('focus-templates-edit').click()
  await page.getByTestId('focus-templates-request').fill(`${templateEditMarker} please delete this template`)
  await page.getByTestId('focus-templates-draft').click()
  await expect(page.getByTestId('focus-templates-preview')).toContainText('lighthouse-story')
  await page.getByTestId('focus-templates-save').click()
  await expect(row).toHaveCount(0)
  listed = await (await page.request.get('/v1/focus/templates')).json()
  expect(listed.templates.map((t: { id: string }) => t.id)).not.toContain('lighthouse-story')
})

test.describe('Korean', () => {
  test.use({ locale: 'ko-KR' })

  test('the template editor chrome is Korean', async ({ page }) => {
    await page.goto('/console/focus/templates')
    await expect(page.getByTestId('focus-templates')).toBeVisible()
    await page.getByTestId('focus-templates-new').click()
    await page.getByTestId('focus-templates-request').fill(`${templateEditMarker} a three-stage story template: outline, draft, revise`)
    await page.getByTestId('focus-templates-draft').click()
    await expect(page.getByTestId('focus-templates-preview')).toBeVisible()

    const chrome = await chromeTexts(page.getByTestId('focus-templates'))
    expect(chrome.length).toBeGreaterThan(5)
    expect(chrome.filter(untranslated)).toEqual([])

    await page.getByTestId('focus-templates-save').click()
    const row = page.locator('[data-testid="focus-templates-row"][data-template-id="lighthouse-story"]')
    await expect(row).toBeVisible()
    await row.getByTestId('focus-templates-edit').click()
    await page.getByTestId('focus-templates-request').fill(`${templateEditMarker} please delete this template`)
    await page.getByTestId('focus-templates-draft').click()
    await page.getByTestId('focus-templates-save').click()
    await expect(row).toHaveCount(0)
  })
})

// A copy of e2e/focus.spec.ts's Korean helpers: each spec file stays
// self-contained rather than importing another spec's internals.
const keptInEnglish = ['TARS', 'Git', 'PR', 'cwd', 'diff', 'Ctrl', 'Cmd', 'Enter', 'Esc']
const englishRun = /[A-Za-z]{2,}[ \t]+[A-Za-z]{2,}/

function untranslated(text: string): boolean {
  let rest = text
  for (const name of keptInEnglish) rest = rest.replaceAll(name, ' ')
  return englishRun.test(rest)
}

async function chromeTexts(scope: ReturnType<Page['getByTestId']>): Promise<string[]> {
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
