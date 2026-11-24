// Vibe mode only (docs/decisions/vibe-coder-identity.md, owner decision
// 2026-11-24): with console_expert_mode off — the default — a person never
// sees or reaches the workbench or focus mode. The e2e server runs with it
// on (e2e/webServers.ts) so every other spec drives the workbench; here
// /v1/healthz answers without it, which is all the console reads to decide.

import { expect, test, type Page } from '@playwright/test'

async function vibeOnly(page: Page) {
  await page.route('**/v1/healthz', async (route) => {
    const real = await route.fetch()
    const body = await real.json()
    return route.fulfill({ response: real, json: { ...body, console_expert_mode: false } })
  })
}

// Nothing in the window that leads into the workbench: no sidebar, no
// expert link, no status pill or notification list in the header, no CASE.
async function expectNoWayOut(page: Page) {
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await expect(page.getByTestId('easy-expert')).toHaveCount(0)
  await expect(page.locator('.status-pill')).toHaveCount(0)
  await expect(page.locator('.header-notif-wrapper')).toHaveCount(0)
  await expect(page.locator('.hamburger-btn')).toHaveCount(0)
  await expect(page.locator('.companion-pet')).toHaveCount(0)
}

test('every workbench and focus address opens vibe mode', async ({ page }) => {
  await vibeOnly(page)
  for (const path of ['/console', '/console/system', '/console/chat', '/console/focus', '/console/focus/templates', '/console/config', '/console/memory']) {
    await page.goto(path)
    await expect(page, path).toHaveURL(/\/console\/easy$/)
    await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
    await expectNoWayOut(page)
  }
  // What a person still needs stays: the language and the header itself.
  await expect(page.getByRole('group', { name: /language/i })).toBeVisible()
})

test('the keyboard opens nothing from the workbench', async ({ page }) => {
  await vibeOnly(page)
  await page.goto('/console/easy')
  await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
  await page.locator('body').click()
  await page.keyboard.press('ControlOrMeta+k')
  await page.keyboard.press('Shift+?')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page).toHaveURL(/\/console\/easy$/)
})

test('going back to a workbench address in the history stays in vibe mode', async ({ page }) => {
  await vibeOnly(page)
  await page.goto('/console/easy')
  await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
  await page.evaluate(() => {
    window.history.pushState(null, '', '/console/chat')
    window.history.pushState(null, '', '/console/easy')
  })
  await page.goBack()
  await expect(page).toHaveURL(/\/console\/easy$/)
  await expectNoWayOut(page)
})

// A stuck project: the expert link is gone, and the sentence does not send
// the person to a mode that is not there.
test('a stuck project offers no expert mode', async ({ page }) => {
  await vibeOnly(page)
  const id = 'feedfacefeedface'
  const project = {
    id, name: 'Book club', idea: 'Book club', folder: '/p', status: 'ready', session_id: 'vibe-only-s1',
    created_at: '2026-11-24T00:00:00Z', updated_at: '2026-11-24T00:00:00Z',
  }
  await page.route(new RegExp(`/v1/easy/projects/${id}$`), (route) => route.fulfill({ json: { project } }))
  await page.route(/\/v1\/focus\/pipelines\/vibe-only-s1$/, (route) =>
    route.fulfill({
      json: {
        version: 1, session_id: 'vibe-only-s1', goal: 'g', current: 'make', cards: [], updated_at: '', template: 'webapp',
        open_gate: 'blocked',
        stages: [
          { id: 'plan', status: 'done', iteration: 1 },
          { id: 'make', kind: 'build', status: 'blocked', iteration: 1 },
          { id: 'review', kind: 'review', status: 'pending', iteration: 0 },
        ],
      },
    }),
  )
  await page.goto(`/console/easy/${id}`)
  await expect(page.getByTestId('easy-phase')).toHaveAttribute('data-phase', 'stuck')
  await expect(page.getByTestId('easy-message')).toHaveText('The team stopped and could not go on by itself. Your idea and what is made so far are kept.')
  await expectNoWayOut(page)
  await expect(page.getByTestId('easy-root')).not.toContainText(/expert/i)
})
