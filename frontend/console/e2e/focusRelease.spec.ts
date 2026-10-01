// Focus mode P5 (docs/decisions/focus-mode.md §9): the release train and
// the pipeline graph. Finishing a whole pipeline through the mock LLM is
// slow, so the release-train listing is stubbed (the server side is covered
// by focus_release_test.go with a real tagged repository); starting the
// release and the graph run against the real server.

import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

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

test('the release train lists merged work and Start release opens a release pipeline behind a confirm', async ({ page }) => {
  const repo = newRepo('tars-e2e-release-')
  await page.route('**/v1/focus/release-train', (route) =>
    route.fulfill({
      json: {
        groups: [
          {
            repo,
            last_tag: 'v0.1.0',
            since: '2026-01-01T00:00:00Z',
            items: [
              { session_id: 'merged-1', title: 'Pipeline graph', goal: 'Draw the pipeline', pr: { number: 42, url: 'https://example.com/pr/42', state: 'merged' }, updated_at: '2026-02-01T00:00:00Z' },
              { session_id: 'merged-2', title: 'Release train', goal: 'List merged work', updated_at: '2026-02-02T00:00:00Z' },
            ],
          },
        ],
      },
    }),
  )

  await page.goto('/console/focus')
  await page.getByTestId('focus-release-open').click()
  await expect(page).toHaveURL(/\/console\/focus\/release$/)
  const group = page.getByTestId('focus-release-group')
  await expect(group).toContainText('since v0.1.0')
  await expect(group).toContainText('2 changes')
  await expect(group.getByRole('link', { name: '#42 Pipeline graph' })).toHaveAttribute('href', 'https://example.com/pr/42')
  await expect(group).toContainText('Release train')

  // Nothing starts before the gate is confirmed.
  await page.getByTestId('focus-release-start').click()
  await expect(page.getByTestId('focus-release-gate')).toContainText('VERSION.txt')
  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByTestId('focus-release-gate')).toHaveCount(0)

  await page.getByTestId('focus-release-start').click()
  await page.getByTestId('focus-release-confirm').click()
  await expect(page).toHaveURL(/\/console\/focus\/(?!release)[^/]+$/)
  const id = decodeURIComponent(page.url().split('/').pop() ?? '')
  const pipeline = await (await page.request.get(`/v1/focus/pipelines/${encodeURIComponent(id)}`)).json()
  expect(pipeline.goal).toMatch(/^Release: ship the 2 change\(s\) merged since v0\.1\.0\./)
  expect(pipeline.goal).toContain('- #42 Pipeline graph (https://example.com/pr/42) — Draw the pipeline')
  expect(pipeline.current).toBe('plan')
})

test('the release train shows its empty state', async ({ page }) => {
  await page.route('**/v1/focus/release-train', (route) => route.fulfill({ json: { groups: [] } }))
  await page.goto('/console/focus/release')
  await expect(page.getByTestId('focus-release-empty')).toBeVisible()
})

test('the pipeline graph opens from the stage bar with plan tasks inside the build node', async ({ page }) => {
  const repo = newRepo('tars-e2e-graph-')
  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: '[e2e:focus-plan] Add a greeting', cwd: repo } })).json()
  const id = created.session_id as string

  await page.goto(`/console/focus/${id}`)
  await expect(page.locator('[data-testid="focus-card"][data-kind="gate"]')).toBeVisible()
  const approved = await page.request.post(`/v1/focus/pipelines/${id}/gates/plan`, { data: { action: 'approve' } })
  expect(approved.ok()).toBeTruthy()
  await page.reload()

  await page.getByTestId('focus-graph-toggle').click()
  const graph = page.getByTestId('focus-graph')
  await expect(graph).toBeVisible()
  await expect(graph.getByTestId('focus-graph-stage')).toHaveCount(6)
  await expect(graph.locator('[data-testid="focus-graph-stage"][data-status="active"]')).toContainText('Build')
  expect(await graph.getByTestId('focus-graph-task').count()).toBeGreaterThan(0)

  await page.getByTestId('focus-graph-toggle').click()
  await expect(graph).toHaveCount(0)
})
