// CASE's sleep (case-companion phase 1.5, F2): the bubble's options menu
// switches CASE off for an hour or until the next alert. Asleep it shows the
// `off` pose with a dark screen, stays asleep across a reload (a per-viewer
// localStorage value), and a click wakes it with the bubble open. The
// wake-on-new-alert rule is checked in tests/companionPet.test.ts.

import { expect, test, type Page } from '@playwright/test'

// Reduced motion keeps the floating button still for Playwright's
// stability check on the click (same as the other companion specs).
test.use({ contextOptions: { reducedMotion: 'reduce' } })

const body = (page: Page) => page.locator('.companion-pet .companion-body')
const bubble = (page: Page) => page.locator('.companion-bubble')
const caseButton = (page: Page) => page.getByRole('button', { name: 'Talk to CASE' })

for (const [option, mode] of [
  ['Sleep for an hour', 'hour'],
  ['Sleep until something needs me', 'next-alert'],
] as const) {
  test(`"${option}" switches CASE off until it is clicked, across a reload`, async ({ page }) => {
    await page.goto('/console')
    await expect(body(page)).toHaveClass(/expr-neutral/)
    await expect(body(page)).not.toHaveClass(/is-asleep/)

    await caseButton(page).click()
    await expect(bubble(page)).toBeVisible()
    await page.getByRole('button', { name: 'CASE options' }).click()
    await page.getByRole('menuitem', { name: option }).click()

    // Asleep: bubble closed, `off` pose, the face and LED dark.
    await expect(bubble(page)).toHaveCount(0)
    await expect(body(page)).toHaveClass(/is-asleep/)
    await expect(page.locator('.companion-pet .companion-eyes')).toBeHidden()
    await expect(page.locator('.companion-pet .companion-gem')).toBeHidden()
    const stored = await page.evaluate(() => window.localStorage.getItem('tars.case.sleep'))
    expect(JSON.parse(stored ?? 'null')?.mode).toBe(mode)

    // Still asleep after a reload.
    await page.reload()
    await expect(body(page)).toHaveClass(/is-asleep/)

    // A click wakes it and opens the bubble.
    await caseButton(page).click()
    await expect(body(page)).not.toHaveClass(/is-asleep/)
    await expect(bubble(page)).toBeVisible()
    await expect(page.locator('.companion-pet .companion-eyes')).toBeVisible()
    expect(await page.evaluate(() => window.localStorage.getItem('tars.case.sleep'))).toBeNull()
  })
}
