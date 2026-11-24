// The workspace's states when the team is not at work
// (docs/plans/vibe-mode-redesign unit 5, screens/D-States), on the real
// server and mock LLM:
//
// - could not start: e2e/fake-scaffold.sh fails an idea that starts with
//   "Scaffold fails once" the first time; Try again starts the same idea as
//   a new project, which works.
// - stuck: [e2e:easy-stuck] adds a check that fails, with its CHECK FAILED
//   report, until the project folder holds .e2e-unstuck. Goal mode spends
//   its budget on it and ends, and the screen shows the failed check with
//   what it expected and saw. Try again with the person's words (carrying
//   [e2e:easy-unstick], which makes the next turn write that file) puts the
//   team back to work on its own, and it finishes.

import { expect, test, type Page } from '@playwright/test'

import { hiddenWordsIn } from '../src/lib/easy.ts'

async function expectNoHiddenWords(page: Page, where: string) {
  const text = await page.getByTestId('easy-root').innerText()
  expect(hiddenWordsIn(text), `${where}:\n${text}`).toEqual([])
}

test('a project that could not be set up starts again on Try again', async ({ page }) => {
  test.setTimeout(120_000)
  await page.goto('/console/easy')
  await page.getByTestId('easy-idea').fill('Scaffold fails once [e2e:focus-template] A guest book for my website')
  await page.getByTestId('easy-start').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)
  const first = page.url()

  const phase = page.getByTestId('easy-phase')
  await expect(phase).toHaveAttribute('data-phase', 'failed', { timeout: 30_000 })
  await expect(page.getByTestId('easy-state')).toHaveAttribute('data-state', 'failed')
  await expect(page.getByTestId('easy-message')).toHaveText('Setting up the project did not work. Check the internet connection and try again.')
  await expectNoHiddenWords(page, 'could not start')

  await page.getByTestId('easy-recreate').click()
  await expect(page).not.toHaveURL(first)
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)
  await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })

  // The failed one left the list.
  const res = await page.request.get(`/v1/easy/projects/${first.split('/').pop()}`)
  expect(res.status()).toBe(404)
})

test('a stuck project shows its failed check and goes on after Try again', async ({ page }) => {
  test.setTimeout(420_000)
  await page.goto('/console/easy')
  await page.getByTestId('easy-idea').fill('[e2e:focus-template] [e2e:easy-stuck] A members list for a book club')
  await page.getByTestId('easy-start').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)

  const phase = page.getByTestId('easy-phase')
  await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })
  await page.getByTestId('easy-approve').click()

  // Goal mode tries again by itself until its budget is spent; only then
  // does the person see the team stuck.
  await expect(phase).toHaveAttribute('data-phase', 'stuck', { timeout: 300_000 })
  await expect(phase).toHaveText('Needs a look')
  await expect(page.getByTestId('easy-state')).toHaveAttribute('data-state', 'stuck')
  await expect(page.getByTestId('easy-message')).toHaveText('It tried to fix one check several times but could not get it to pass.')
  const check = page.getByTestId('easy-stuck-check')
  await expect(check).toContainText('The newest member is at the top of the list')
  await expect(check.getByTestId('easy-check-report')).toContainText('the newest member at the top')
  await expect(check.getByTestId('easy-check-report')).toContainText('the newest member at the bottom')
  await expect(page.getByTestId('easy-stuck-count')).toHaveText('2 of 3 checks passed')
  await expectNoHiddenWords(page, 'stuck')

  await page.getByTestId('easy-stuck-note').fill('The newest member goes at the top of the list [e2e:easy-unstick]')
  await page.getByTestId('easy-retry').click()
  await expect(phase).not.toHaveAttribute('data-phase', 'stuck', { timeout: 30_000 })
  // Nothing else is clicked: the team goes on by itself to the end.
  await expect(phase).toHaveAttribute('data-phase', 'finished', { timeout: 180_000 })
})
