// Sidebar navigation groups (#968).

import { expect, test } from '@playwright/test'

const nav = (page: import('@playwright/test').Page) => page.getByRole('navigation', { name: 'Main navigation' })

test('the nav shows Work, Build, and System groups and navigates', async ({ page }) => {
  await page.goto('/console/chat')
  for (const group of ['Work', 'Build', 'System']) {
    await expect(nav(page).locator('.nav-group-label', { hasText: group })).toBeVisible()
  }
  await nav(page).getByRole('link', { name: 'Memory' }).click()
  await expect(page).toHaveURL(/\/console\/memory$/)
  await expect(nav(page).getByRole('link', { name: 'Memory' })).toHaveClass(/active/)
  await expect(nav(page).getByRole('link', { name: 'Chat' })).not.toHaveClass(/active/)
})

test('a legacy alias highlights the item for the page it resolves to', async ({ page }) => {
  await page.goto('/console/ops')
  await expect(nav(page).getByRole('link', { name: 'Approvals' })).toHaveClass(/active/)
})
