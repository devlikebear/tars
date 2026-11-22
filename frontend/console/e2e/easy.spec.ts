// Easy mode (docs/decisions/vibe-coder-identity.md §4, phase 2): one sentence
// leads to a finished, saved project with nothing clicked but "Make it like
// this", and no screen on the way shows the machinery underneath (git, pull
// requests, pipelines, gates…). The real server runs the whole flow: the
// project is made under the e2e workspace by e2e/fake-scaffold.sh, the mock
// LLM plays the webapp template ([e2e:focus-template]: a plan, a high and a
// low finding, then none), and goal mode takes over once the plan is
// approved — the high finding is fixed, the low one set aside.

import { expect, test, type Page } from '@playwright/test'

import { hiddenWordsIn } from '../src/lib/easy.ts'

const idea = '[e2e:focus-template] A sign-up page for a book club'

// The hidden words on the easy screen right now, read from its own root (the
// header and CASE around it are not easy mode's to word).
async function expectNoHiddenWords(page: Page, where: string) {
  const text = await page.getByTestId('easy-root').innerText()
  expect(hiddenWordsIn(text), `${where}:\n${text}`).toEqual([])
}

test.describe.serial('easy mode', () => {
  let projectPath = ''

  test('one sentence becomes a finished, saved project', async ({ page }) => {
    test.setTimeout(150_000)
    await page.goto('/console/easy')
    await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
    // The workbench's navigation is not part of easy mode.
    await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
    await expectNoHiddenWords(page, 'make screen')

    // An empty idea is refused in plain words, before anything is made.
    await page.getByTestId('easy-start').click()
    await expect(page.getByTestId('easy-error')).toHaveText('Write what you want to make first.')

    await page.getByTestId('easy-idea').fill(idea)
    await page.getByTestId('easy-start').click()
    await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)
    projectPath = new URL(page.url()).pathname

    const phase = page.getByTestId('easy-phase')
    await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })
    await expectNoHiddenWords(page, 'plan to approve')
    const yourTurn = page.getByTestId('easy-your-turn')
    await expect(yourTurn).toContainText('Write a short story')
    await expect(yourTurn).toContainText('Write chapter one')
    await expect(page.getByTestId('easy-bot-leader')).toHaveAttribute('data-state', 'needs_you')
    await expect(page.getByTestId('easy-bot-publisher')).toHaveAttribute('data-state', 'later')

    // The person's only click. From here goal mode decides.
    await page.getByTestId('easy-approve').click()
    await expect(yourTurn).toHaveCount(0)
    await expect(phase).not.toHaveAttribute('data-phase', 'approve')
    await expectNoHiddenWords(page, 'being made')

    await expect(phase).toHaveAttribute('data-phase', 'finished', { timeout: 120_000 })
    await expect(page.getByTestId('easy-message')).toHaveText('Finished, and saved as a version.')
    for (const bot of ['leader', 'maker', 'reviewer', 'checker']) {
      await expect(page.getByTestId(`easy-bot-${bot}`)).toHaveAttribute('data-state', 'done')
    }
    await expectNoHiddenWords(page, 'finished')

    // The finished app runs on this computer and shows in place
    // (e2e/fake-preview.sh stands in for its development server).
    const frame = page.getByTestId('easy-preview-frame')
    await expect(frame).toHaveAttribute('src', /^http:\/\/127\.0\.0\.1:\d+\/$/, { timeout: 30_000 })
    await expect(page.frameLocator('[data-testid="easy-preview-frame"]').getByRole('heading', { name: 'Book club sign-up' })).toBeVisible()
    await expectNoHiddenWords(page, 'result')

    // The project is listed on the make screen, and expert mode opens its
    // work in the focus screen.
    await page.getByRole('button', { name: /All projects/ }).click()
    await expect(page.getByTestId('easy-project-item')).toContainText('A sign-up page for a book club')
    await page.goto(projectPath)
    await page.getByTestId('easy-expert').click()
    await expect(page).toHaveURL(/\/console\/focus\/[^/]+$/)
  })

  test.describe('Korean', () => {
    test.use({ locale: 'ko-KR' })

    test('the easy screens read in Korean and still hide the machinery', async ({ page }) => {
      await page.goto('/console/easy')
      await expect(page.getByRole('heading', { name: '무엇을 만들까요?' })).toBeVisible()
      await expectNoHiddenWords(page, 'make screen (ko)')
      expect(projectPath, 'the English test made a project').not.toBe('')
      await page.goto(projectPath)
      await expect(page.getByTestId('easy-message')).toHaveText('완성했고, 버전으로 저장했어요.')
      await expect(page.getByTestId('easy-bot-publisher')).toContainText('출판봇')
      await expect(page.getByTestId('easy-result')).toContainText('내 앱')
      await expect(page.getByTestId('easy-preview-frame')).toBeVisible()
      await expectNoHiddenWords(page, 'finished (ko)')
    })
  })
})
