// The vibe-mode workspace (docs/plans/vibe-mode-redesign unit 4) on the real
// server and mock LLM: the leader asks its questions before the plan
// ([e2e:easy-ask]) and they are answered on the screen, the blueprint shows
// its screens, checks, what is kept and what is left out, the person asks
// for a change in it and then approves, and the team makes and checks it
// to the end. [e2e:easy-check-fail] adds a check whose command fails with
// its own CHECK FAILED report for its first twelve seconds, so the screen
// shows a failed check with what it expected and saw before every check
// passes.

import { expect, test, type Page } from '@playwright/test'

import { hiddenWordsIn } from '../src/lib/easy.ts'

async function expectNoHiddenWords(page: Page, where: string) {
  const text = await page.getByTestId('easy-root').innerText()
  expect(hiddenWordsIn(text), `${where}:\n${text}`).toEqual([])
}

test('questions, blueprint, making and checking on the workspace', async ({ page }) => {
  test.setTimeout(300_000)
  await page.goto('/console/easy')
  await page.getByTestId('easy-idea').fill('[e2e:focus-template] [e2e:easy-ask] [e2e:easy-check-fail] A sign-up page for a book club')
  await page.getByTestId('easy-start').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)

  // The leader's questions, each with the server's "you decide" choice.
  const phase = page.getByTestId('easy-phase')
  await expect(phase).toHaveAttribute('data-phase', 'questions', { timeout: 60_000 })
  await expect(page.getByTestId('easy-step-plan')).toHaveAttribute('data-state', 'now')
  await expect(page.getByTestId('easy-bot-leader')).toHaveAttribute('data-state', 'needs_you')
  const questions = page.getByTestId('easy-question')
  await expect(questions).toHaveCount(2)
  await expect(questions.nth(0)).toContainText('Who can see the member list?')
  await expect(questions.nth(0).getByTestId('easy-question-choice')).toHaveText(['Everyone', 'Only members', 'You decide'])
  await expect(page.getByTestId('easy-log-you')).toContainText('A sign-up page for a book club')
  await expectNoHiddenWords(page, 'questions')

  await questions.nth(0).getByRole('button', { name: 'Only members' }).click()
  await expect(questions.nth(0)).toHaveAttribute('data-answered', 'true')
  await expect(questions.nth(0).getByRole('button', { name: 'Only members' })).toHaveAttribute('aria-pressed', 'true')
  await expect(phase).toHaveAttribute('data-phase', 'questions')
  await questions.nth(1).getByRole('button', { name: 'You decide' }).click()

  // The answers go to the leader, who draws the blueprint.
  await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })
  const blueprint = page.getByTestId('easy-your-turn')
  await expect(blueprint).toContainText('Write a short story')
  await expect(page.getByTestId('easy-sketch')).toContainText('Sign-up')
  await expect(page.getByTestId('easy-sketch')).toContainText('Join the book club')
  await expect(page.getByTestId('easy-blueprint-checks')).toContainText('The page opens')
  await expect(page.getByTestId('easy-blueprint-checks')).toContainText('The newest member is at the top of the list')
  await expect(page.getByTestId('easy-blueprint-saved')).toContainText('The names and emails of members')
  await expect(page.getByTestId('easy-blueprint-not-now')).toContainText('Sending emails to members')
  await expect(page.getByTestId('easy-question').nth(1).getByRole('button', { name: 'You decide' })).toHaveAttribute('aria-pressed', 'true')
  await expectNoHiddenWords(page, 'blueprint')

  // Telling the team what to change sends the leader back to the drawing.
  await page.getByTestId('easy-say-send').click()
  await expect(page.getByTestId('easy-say-error')).toHaveText('Write what to change first.')
  await page.getByTestId('easy-say-input').fill('Show the meeting date too')
  await page.getByTestId('easy-say-send').click()
  await expect(phase).not.toHaveAttribute('data-phase', 'approve', { timeout: 30_000 })
  await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })

  // The one click. From here the team goes on by itself.
  await page.getByTestId('easy-approve').click()
  await expect(page.getByTestId('easy-step-plan')).toHaveAttribute('data-state', 'done', { timeout: 30_000 })
  await expect(page.getByTestId('easy-say')).toHaveCount(0)

  // A failed check, as the server recorded it: what it expected and saw.
  const report = page.getByTestId('easy-check-report')
  await expect(report).toBeVisible({ timeout: 60_000 })
  await expect(report).toContainText('the newest member at the top')
  await expect(report).toContainText('the newest member at the bottom')
  await expect(page.getByTestId('easy-step-check')).toHaveAttribute('data-state', 'now')
  const rows = page.getByTestId('easy-check-row')
  await expect(rows).toHaveCount(3)
  await expect(rows.nth(2)).toHaveAttribute('data-state', 'failed')
  await expect(rows.nth(2)).toContainText('Members list')
  await expectNoHiddenWords(page, 'a failed check')

  // Every check passes in the end, and the team finishes.
  await expect(phase).toHaveAttribute('data-phase', 'finished', { timeout: 180_000 })
  await expect(page.getByTestId('easy-log')).toContainText('All 3 checks passed.')
  for (const step of ['plan', 'make', 'check']) {
    await expect(page.getByTestId(`easy-step-${step}`)).toHaveAttribute('data-state', 'done')
  }
  await expectNoHiddenWords(page, 'finished')
})
