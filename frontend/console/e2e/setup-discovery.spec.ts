import { expect, test, type Page } from '@playwright/test'
const claude = { kind: 'claude-code-cli', installed: true, ready: true, models: ['opus','sonnet','haiku'], source: 'cli', recommended: { heavy: 'opus', standard: 'sonnet', light: 'haiku' } }
const codex = { kind: 'openai-codex', installed: true, ready: true, models: ['latest-model'], source: 'live', recommended: { heavy: 'latest-model', standard: 'latest-model', light: 'latest-model' } }
async function openWizard(page: Page, candidates: unknown[], reentry = false) {
  await page.addInitScript(() => localStorage.setItem('tars_console_locale','en'))
  await page.route('**/v1/admin/setup/discover', route => route.fulfill({json:{candidates}}))
  await page.goto(`/console/onboarding${reentry ? '?reentry=1' : ''}`)
  await expect(page.getByText('Use an existing sign-in')).toBeVisible()
}
test('first run fills the only ready provider and models without saving', async ({page})=>{
  let writes=0
  await page.route('**/v1/admin/config/values',async route=>{if(route.request().method()==='PATCH') writes++;await route.fallback()})
  await openWizard(page,[claude,{...codex,ready:false}])
  await expect(page.getByText('One ready provider was found and filled in. Review or change it below.')).toBeVisible()
  await expect(page.locator('.onboarding-grid select').first()).toHaveValue('claude-code-cli')
  await page.getByRole('button',{name:/Next.*Tier/i}).click()
  await expect(page.locator('.onboarding-tier').filter({has:page.locator('legend',{hasText:'standard'})}).locator('input[list]')).toHaveValue('sonnet')
  expect(writes).toBe(0)
})
test('two ready providers wait for user choice and recommend live Codex models', async ({page})=>{
  await openWizard(page,[claude,codex])
  await expect(page.getByText('Choose the provider you want to use.',{exact:false})).toBeVisible()
  await expect(page.locator('.onboarding-grid select').first()).toHaveValue('')
  await page.locator('.setup-discovery-candidate').filter({has:page.getByText('Codex',{exact:true})}).getByRole('button',{name:'Use this provider'}).click()
  await page.getByRole('button',{name:/Next.*Tier/i}).click()
  await expect(page.locator('.onboarding-tier input[list]')).toHaveCount(3)
  for(const input of await page.locator('.onboarding-tier input[list]').all()) await expect(input).toHaveValue('latest-model')
  await expect(page.getByText('Models refreshed from the provider.')).toBeVisible()
})
test('unavailable tools show installation and login guidance', async ({page})=>{
  await openWizard(page,[{...claude,installed:false,ready:false,problem:'cli_missing'},{...codex,ready:false,problem:'not_logged_in'}])
  await expect(page.getByRole('link',{name:'Install instructions'})).toHaveAttribute('href','https://code.claude.com/docs/en/setup')
  await expect(page.locator('.setup-discovery code')).toContainText('cli_auth_credentials_store="file"')
  await expect(page.locator('.onboarding-grid select').first()).toHaveValue('')
})
test('re-entry keeps existing provider and manual tiers', async ({page})=>{
  await openWizard(page,[claude],true)
  await expect(page.locator('.onboarding-grid select').first()).toHaveValue('openai')
  await page.getByRole('button',{name:/Next.*Tier/i}).click()
  for(const input of await page.locator('.onboarding-tier input[list]').all()) await expect(input).toHaveValue('e2e-model')
})
