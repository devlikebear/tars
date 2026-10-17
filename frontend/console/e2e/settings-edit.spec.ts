import { expect, test } from '@playwright/test'

test('settings edits and saves individual fields without wizard', async ({ page, request }) => {
  await page.addInitScript(() => localStorage.setItem('tars_console_locale', 'en'))
  await page.goto('/console/config')
  await page.getByRole('searchbox', { name: 'Search settings' }).fill('log_level')
  const original = await request.get('/v1/admin/config/schema').then(r => r.json())
  await page.locator('.config-search-hit', { hasText: 'Log Level' }).click()
  const select = page.locator('#config-field-log_level select')
  await expect(select).toBeVisible()
  await select.selectOption('debug')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved; restart required to apply. Environment overrides take precedence.').first()).toBeVisible()
  const saved = await request.get('/v1/admin/config/schema').then(r => r.json())
  expect(saved.values.log_level).toBe('debug')
  expect(saved.runtime_values.log_level).toBe(original.runtime_values.log_level)
  expect(saved.pending_restart_keys).toContain('log_level')
  const invalid = await request.patch('/v1/admin/config/values', { data: { updates: { log_level: true } } })
  expect(invalid.status()).toBe(400)
  await request.patch('/v1/admin/config/values', { data: { updates: { log_level: original.values.log_level } } })
})

// Regression for the <input type="number"> + bind:value crash: Svelte
// coerces that binding's value to a number (or null), which broke the
// control's string-only draft.trim() parsing on commit — so typing into
// any int/float field and leaving it never saved (tars PR #1283 review).
test('an int field commits through the number input without throwing', async ({ page, request }) => {
  await page.addInitScript(() => localStorage.setItem('tars_console_locale', 'en'))
  await page.goto('/console/config')
  const original = await request.get('/v1/admin/config/schema').then(r => r.json())
  const before = Number(original.values.session_auto_archive_days ?? 0)
  const next = before === 30 ? 45 : 30
  await page.getByRole('searchbox', { name: 'Search settings' }).fill('session_auto_archive_days')
  await page.locator('.config-search-hit', { hasText: 'Auto-archive' }).click()
  const numberInput = page.locator('#config-field-session_auto_archive_days input[type="number"]')
  await expect(numberInput).toBeVisible()
  await numberInput.fill(String(next))
  await numberInput.blur()
  await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved; restart required to apply. Environment overrides take precedence.').first()).toBeVisible()
  const saved = await request.get('/v1/admin/config/schema').then(r => r.json())
  expect(saved.values.session_auto_archive_days).toBe(next)
  await request.patch('/v1/admin/config/values', { data: { updates: { session_auto_archive_days: before } } })
})
