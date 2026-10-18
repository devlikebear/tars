// CASE's Bridge (case-companion phase 2): Mod+/ opens the panel on any
// screen; a question runs in CASE's own hidden session with the server's
// <tars-status> turn context, streams back as text, and ends with the cards
// the server checked against the same snapshot — a card naming something
// not in it is dropped. The mock LLM (e2e/mock-llm.mjs, [e2e:bridge]) says
// whether the status reached it.

import { expect, test, type Page } from '@playwright/test'

// Reduced motion keeps CASE's floating button still for clicks.
test.use({ contextOptions: { reducedMotion: 'reduce' } })

const panel = (page: Page) => page.getByRole('dialog', { name: 'Bridge' })

test('Mod+/ opens the Bridge, which answers from the status with checked cards', async ({ page }) => {
  await page.goto('/console/ops')
  await expect(page.locator('.companion-pet .companion-body')).toBeVisible()

  await page.keyboard.press('ControlOrMeta+/')
  await expect(panel(page)).toBeVisible()
  await expect(panel(page)).toContainText('Looking at: Ops')
  // No session on this screen: "This screen" cannot be chosen.
  await expect(panel(page).getByRole('radio', { name: 'This screen' })).toBeDisabled()

  const input = panel(page).getByRole('textbox', { name: 'Ask CASE about TARS' })
  await expect(input).toBeFocused()
  await input.fill('[e2e:bridge] is anything stuck?')
  await input.press('Enter')

  // The panel brings back the server's last answer when it opens, so
  // only the newest exchange is this question's.
  const latest = panel(page).locator('.case-bridge-entry').last()
  await expect(latest.locator('.case-bridge-question')).toHaveText('[e2e:bridge] is anything stuck?')
  await expect(latest.locator('.case-bridge-answer')).toHaveText('Status seen. Nothing is stuck.')
  // The card block never shows as text.
  await expect(panel(page)).not.toContainText('case-cards')
  const cards = latest.locator('.case-bridge-card')
  await expect(cards).toHaveCount(1)
  await expect(cards.first()).toContainText('Spend today')
  await expect(panel(page)).not.toContainText('Invented')

  // A card goes to its screen and closes the panel.
  await cards.first().click()
  await expect(page).toHaveURL(/\/console\/analytics$/)
  await expect(panel(page)).toHaveCount(0)

  // Opening again keeps this page's conversation; Escape closes.
  await page.keyboard.press('ControlOrMeta+/')
  await expect(panel(page).locator('.case-bridge-question').last()).toHaveText('[e2e:bridge] is anything stuck?')
  await page.keyboard.press('Escape')
  await expect(panel(page)).toHaveCount(0)
})

test("CASE's bubble opens the Bridge, and the Bridge question never lands in the main session", async ({ page }) => {
  await page.goto('/console')
  await page.getByRole('button', { name: 'Talk to CASE' }).click()
  await page.getByRole('button', { name: 'CASE options' }).click()
  await page.getByRole('menuitem', { name: 'Ask about TARS' }).click()
  await expect(page.locator('.companion-bubble')).toHaveCount(0)
  await expect(panel(page)).toBeVisible()

  const input = panel(page).getByRole('textbox', { name: 'Ask CASE about TARS' })
  await input.fill('[e2e:bridge] private question')
  await panel(page).getByRole('button', { name: 'Ask' }).click()
  await expect(panel(page).locator('.case-bridge-answer').last()).toHaveText('Status seen. Nothing is stuck.')

  // The main session's transcript does not have it; CASE's session is
  // hidden from the session list.
  const sessions = (await page.request.get('/v1/admin/sessions').then((r) => r.json())) as { id: string; kind?: string; title?: string }[]
  const main = sessions.find((s) => s.kind === 'main')
  expect(main).toBeTruthy()
  const history = await page.request.get(`/v1/admin/sessions/${main!.id}/history`).then((r) => r.text())
  expect(history).not.toContain('private question')
  expect(sessions.some((s) => s.title === 'CASE Bridge')).toBe(false)
  // It is there with hidden sessions included.
  const all = (await page.request.get('/v1/admin/sessions?hidden=1').then((r) => r.json())) as { title?: string }[]
  expect(all.some((s) => s.title === 'CASE Bridge')).toBe(true)
})
