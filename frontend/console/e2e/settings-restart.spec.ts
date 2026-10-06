import { expect, test } from '@playwright/test'

test('saved settings apply only after explicit restart confirmation', async ({ page, request }) => {
  test.setTimeout(120_000)
  await page.addInitScript(() => localStorage.setItem('tars_console_locale','en'))
  await page.goto('/console/config')
  const before = await request.get('/v1/admin/config/schema').then(r => r.json())
  await page.getByRole('textbox',{name:'Search settings'}).fill('log_level')
  const card = page.locator('.quick-start-card').filter({hasText:'Log Level'})
  const next = before.values.log_level === 'warn' ? 'info' : 'warn'
  await card.locator('select').selectOption(next)
  await page.getByRole('button',{name:'Save and apply',exact:true}).click()
  await expect(page.getByRole('button',{name:'Confirm restart',exact:true})).toBeVisible()
  const saved = await request.get('/v1/admin/config/schema').then(r=>r.json())
  expect(saved.runtime_started_at).toBe(before.runtime_started_at)
  await page.getByRole('button',{name:'Confirm restart',exact:true}).click()
  await expect.poll(async () => {
    try { return await request.get('/v1/admin/config/schema').then(r=>r.json()).then(s=>s.runtime_values.log_level) } catch { return '' }
  },{timeout:60_000}).toBe(next)
  const after = await request.get('/v1/admin/config/schema').then(r=>r.json())
  expect(after.runtime_started_at).not.toBe(before.runtime_started_at)
  await request.patch('/v1/admin/config/values',{data:{updates:{log_level:before.values.log_level}}})
})
