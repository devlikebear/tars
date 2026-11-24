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

const idea = '[e2e:focus-template] [e2e:easy-write] A sign-up page for a book club'

// The hidden words on the easy screen right now, read from its own root (the
// header and CASE around it are not easy mode's to word).
async function expectNoHiddenWords(page: Page, where: string) {
  const text = await page.getByTestId('easy-root').innerText()
  expect(hiddenWordsIn(text), `${where}:\n${text}`).toEqual([])
}

test.describe.serial('easy mode', () => {
  let projectPath = ''

  test('one sentence becomes a finished, saved project', async ({ page }) => {
    test.setTimeout(300_000)
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

    // "Change it like this": an empty request is refused in plain words; a
    // real one opens the next run in the same project, whose plan waits for
    // the person again and shows what they asked for.
    await page.getByTestId('easy-change-submit').click()
    await expect(page.getByTestId('easy-change-error')).toHaveText('Write what you want to change first.')
    await page.getByTestId('easy-change-input').fill('[e2e:focus-template] [e2e:easy-write] Make the sign-up button green')
    await page.getByTestId('easy-change-submit').click()
    await expect(page.getByTestId('easy-change')).toHaveCount(0)
    await expect(page.getByTestId('easy-asked')).toContainText('Make the sign-up button green')
    await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })
    await expectNoHiddenWords(page, 'change plan to approve')
    await page.getByTestId('easy-approve').click()
    await expect(phase).toHaveAttribute('data-phase', 'finished', { timeout: 120_000 })
    await expect(page.getByTestId('easy-message')).toHaveText('Finished, and saved as a version.')
    await expect(page.getByTestId('easy-change')).toBeVisible()
    await expectNoHiddenWords(page, 'change finished')

    // Versions: the change, the first version, the empty start — newest
    // first. Going back to the first version saves it again as the newest,
    // keeps the others, and starts the preview again.
    const versions = page.getByTestId('easy-version')
    await expect(versions).toHaveCount(3)
    await expect(versions.nth(0)).toHaveAttribute('data-current', 'true')
    await expect(versions.nth(2)).toContainText('The empty starting project')
    await versions.nth(1).getByTestId('easy-version-restore').click()
    await expect(versions).toHaveCount(4)
    await expect(versions.nth(0)).toHaveAttribute('data-current', 'true')
    await expect(versions.nth(0)).toContainText('Back to: Write a short story')
    await expect(page.getByTestId('easy-preview-frame')).toBeVisible({ timeout: 30_000 })
    await expectNoHiddenWords(page, 'went back')

    // The project is listed on the make screen, and expert mode opens its
    // work in the focus screen.
    await page.getByRole('button', { name: /All projects/ }).click()
    await expect(page.getByTestId('easy-project-item')).toContainText('A sign-up page for a')
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
      await expect(page.getByTestId('easy-change')).toContainText('이렇게 고쳐 줘')
      await expect(page.getByTestId('easy-version').first()).toContainText('되돌림: Write a short story')
      await expect(page.getByTestId('easy-versions')).toContainText('처음 만든 빈 프로젝트')
      await expectNoHiddenWords(page, 'finished (ko)')
    })
  })
})

// The Start screen (phase 4): while the computer lacks what a project needs,
// it takes the make box's place and explains only the next thing to do. The
// e2e server itself is ready (a mock engine, the stand-in scaffold), so the
// not-ready answers are played by the browser.
// The AI's usage limit (phase 4): once the plan is approved and goal mode
// works on its own, the mock refuses the first make turn the way Claude
// Code words a spent session limit. Goal mode stops instead of retrying; the
// screen says so in plain words with the time it comes back, never the
// error, and Continue sends the turn again and lets the team finish.
test('a spent usage limit pauses the project until the person continues', async ({ page }) => {
  test.setTimeout(180_000)
  await page.goto('/console/easy')
  await page.getByTestId('easy-idea').fill('[e2e:focus-template] [e2e:easy-limit] A guest book for my website')
  await page.getByTestId('easy-start').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)

  const phase = page.getByTestId('easy-phase')
  await expect(phase).toHaveAttribute('data-phase', 'approve', { timeout: 60_000 })
  await page.getByTestId('easy-approve').click()

  await expect(phase).toHaveAttribute('data-phase', 'limit', { timeout: 60_000 })
  await expect(phase).toHaveText('Paused')
  const message = page.getByTestId('easy-message')
  await expect(message).toContainText('used up what your plan allows')
  await expect(message).toContainText('2:40pm (Asia/Seoul)')
  await expect(page.getByTestId('easy-root')).not.toContainText('session limit')
  await expectNoHiddenWords(page, 'usage limit')

  await page.getByTestId('easy-continue').click()
  await expect(page.getByTestId('easy-limit')).toHaveCount(0)
  // Nothing else is clicked: the team goes on by itself to the end.
  await expect(phase).toHaveAttribute('data-phase', 'finished', { timeout: 120_000 })
})

test('the Start screen shows the next step until the computer is ready', async ({ page }) => {
  const real = await page.request.get('/v1/easy/readiness')
  expect((await real.json()).readiness).toMatchObject({ ready: true, engine: 'ok', tools: 'ok' })

  const answers = [
    { ready: false, next: 'install_engine', engine: 'missing', engine_kind: 'claude-code-cli', tools: 'missing' },
    { ready: false, next: 'sign_in', engine: 'signed_out', engine_kind: 'claude-code-cli', tools: 'ok' },
    { ready: true, engine: 'ok', engine_kind: 'claude-code-cli', tools: 'ok' },
  ]
  await page.route('**/v1/easy/readiness', (route) => route.fulfill({ json: { readiness: answers.shift() ?? { ready: true, engine: 'ok', tools: 'ok' } } }))
  await page.goto('/console/easy')

  const next = page.getByTestId('easy-start-next')
  await expect(next).toHaveAttribute('data-next', 'install_engine')
  await expect(next).toContainText('Install Claude Code')
  await expect(page.getByTestId('easy-start-command')).toHaveText('curl -fsSL https://claude.ai/install.sh | bash')
  await expect(page.getByTestId('easy-start-tools')).toHaveAttribute('data-state', 'missing')
  await expect(page.getByTestId('easy-idea')).toHaveCount(0)
  await expectNoHiddenWords(page, 'start screen')

  await page.getByTestId('easy-start-check').click()
  await expect(next).toHaveAttribute('data-next', 'sign_in')
  await expect(page.getByTestId('easy-start-engine')).toHaveAttribute('data-state', 'signed_out')

  // The next answer is read by itself, without a click.
  await expect(page.getByTestId('easy-idea')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('easy-start-screen')).toHaveCount(0)
})

// Easy mode's first run (phase 4-2): a server that still needs setup does
// not send easy mode to the wizard. The screen sets Claude Code up itself
// and waits for the restart. The e2e server is already set up, so the
// browser plays one that is not until the setup call arrives.
test('easy mode sets TARS up on its first run instead of opening the wizard', async ({ page }) => {
  let phase: 'needs' | 'restarting' | 'done' = 'needs'
  let setUps = 0
  await page.route('**/v1/healthz', async (route) => {
    const real = await route.fetch()
    const body = await real.json()
    if (phase === 'done') return route.fulfill({ response: real, json: body })
    // One more answer that still needs setup, as a restart would give.
    if (phase === 'restarting') phase = 'done'
    return route.fulfill({ response: real, json: { ...body, needs_setup: true } })
  })
  await page.route('**/v1/admin/setup/easy', (route) => {
    setUps++
    phase = 'restarting'
    return route.fulfill({ json: { ok: 'true', mode: 'exec', info: 're-executing process' } })
  })

  await page.goto('/console/easy')
  await expect(page).toHaveURL(/\/console\/easy$/)
  await expect(page.getByTestId('easy-setup')).toBeVisible()
  await expect(page.getByText('Setup-only mode')).toHaveCount(0)
  await expectNoHiddenWords(page, 'first-run setup')
  await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible({ timeout: 15_000 })
  expect(setUps).toBe(1)
  await expect(page).not.toHaveURL(/onboarding/)
})
