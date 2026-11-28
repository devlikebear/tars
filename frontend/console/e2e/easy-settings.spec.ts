// Vibe mode's settings screen (/console/easy/settings): the phone, the AI
// and expert mode, reached from the vibe shell with expert mode off. Remote
// access and config writes are answered here, never by the shared server:
// a real turn-on would touch the host's Tailscale, and a real config write
// would leave a pending restart for every spec after this one.

import { expect, test, type Page } from '@playwright/test'

import { hiddenWordsIn } from '../src/lib/easy.ts'

async function vibeOnly(page: Page) {
  await page.route('**/v1/healthz', async (route) => {
    const real = await route.fetch()
    const body = await real.json()
    return route.fulfill({ response: real, json: { ...body, console_expert_mode: false } })
  })
}

const remote = (status: Record<string, boolean>, failed: string[] = [], url = '') => ({
  desired_enabled: Boolean(status.serve_active),
  desired_https_port: 443,
  target_url: 'http://127.0.0.1:43180',
  url,
  status,
  checks: ['api_auth_required', 'admin_password_configured', 'user_password_configured', 'tailscale_installed', 'tailscale_logged_in', 'serve_target_available'].map(
    (key) => ({ key, ok: !failed.includes(key), message: key }),
  ),
})

async function plainWords(page: Page) {
  const text = await page.getByTestId('easy-settings').innerText()
  expect(hiddenWordsIn(text), text).toEqual([])
}

test('the vibe shell opens settings, and turning on phone access lets the phone pair', async ({ page }) => {
  await vibeOnly(page)
  let serving = false
  await page.route('**/v1/admin/remote-access/status', (route) =>
    route.fulfill({
      json: serving
        ? remote({ installed: true, logged_in: true, serve_active: true, owned_by_tars: true }, [], 'https://desk.example.ts.net')
        : remote({ installed: true, logged_in: true }),
    }),
  )
  await page.route('**/v1/admin/remote-access/enable', (route) => {
    serving = true
    return route.fulfill({ json: remote({ installed: true, logged_in: true, serve_active: true, owned_by_tars: true }, [], 'https://desk.example.ts.net') })
  })
  await page.route('**/v1/admin/devices/pairing-code', (route) =>
    route.fulfill({ json: { code: 'ABCD-1234', role: 'focus', expires_at: new Date(Date.now() + 600_000).toISOString() } }),
  )

  await page.goto('/console/easy')
  await page.getByTestId('vibe-nav-settings').click()
  await expect(page).toHaveURL(/\/console\/easy\/settings$/)
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible()
  await expect(page.getByTestId('vibe-nav-settings')).toHaveAttribute('aria-current', 'page')

  // Phone access is off: the card cannot pair yet and says why.
  await expect(page.getByTestId('easy-settings-access')).toHaveAttribute('data-access', 'off')
  await expect(page.getByTestId('phones-pair')).toBeDisabled()
  await expect(page.getByTestId('phones-card')).toContainText('Turn on phone access above first')
  // The vibe card leaves out the folders a focus task starts in.
  await expect(page.getByTestId('phones-add-folder')).toHaveCount(0)
  await plainWords(page)

  await page.getByTestId('easy-settings-access-on').click()
  await expect(page.getByTestId('easy-settings-access')).toHaveAttribute('data-access', 'on')
  await expect(page.getByTestId('easy-settings-access')).toContainText('https://desk.example.ts.net')
  await page.getByTestId('phones-pair').click()
  await expect(page.getByTestId('phones-code')).toHaveText('ABCD-1234')
  await expect(page.getByRole('img', { name: 'Pairing QR code' })).toBeVisible()
})

test('phone access says plainly what blocks it', async ({ page }) => {
  await vibeOnly(page)
  let answer = remote({}, ['tailscale_installed', 'tailscale_logged_in'])
  await page.route('**/v1/admin/remote-access/status', (route) => route.fulfill({ json: answer }))
  await page.goto('/console/easy/settings')
  await expect(page.getByTestId('easy-settings-access')).toHaveAttribute('data-access', 'noTailscale')
  await expect(page.getByRole('link', { name: 'Get Tailscale' })).toHaveAttribute('href', 'https://tailscale.com/download')

  answer = remote({ installed: true, logged_in: true }, ['api_auth_required'])
  await page.getByRole('button', { name: 'Check again' }).click()
  await expect(page.getByTestId('easy-settings-access')).toContainText('ask for a password first')
  await expect(page.getByTestId('easy-settings-access-on')).toHaveCount(0)
  await plainWords(page)
})

test('choosing how the team thinks saves and offers a restart, never restarting by itself', async ({ page }) => {
  await vibeOnly(page)
  await page.route('**/v1/admin/remote-access/status', (route) => route.fulfill({ json: remote({}) }))
  let saved: Record<string, unknown> | null = null
  let restarts = 0
  await page.route('**/v1/admin/config/schema', async (route) => {
    const real = await route.fetch()
    const body = await real.json()
    const tiers = {
      heavy: { provider: 'claude', model: 'opus' },
      standard: { provider: 'claude', model: 'sonnet' },
      light: { provider: 'claude', model: 'haiku' },
    }
    const values = {
      ...body.values,
      llm_providers: { claude: { kind: 'claude-code-cli', auth_mode: 'cli' } },
      llm_tiers: tiers,
      llm_role_defaults: {},
      llm_default_tier: 'standard',
      ...(saved ?? {}),
    }
    return route.fulfill({
      response: real,
      json: { ...body, values, effective_values: values, pending_restart_keys: saved ? Object.keys(saved) : [] },
    })
  })
  await page.route('**/v1/admin/config/values', async (route) => {
    saved = (route.request().postDataJSON() as { updates: Record<string, unknown> }).updates
    return route.fulfill({ json: { ok: 'saved' } })
  })
  await page.route('**/v1/admin/restart', (route) => {
    restarts++
    return route.fulfill({ json: { ok: 'restarting', mode: 'test', info: '' } })
  })

  await page.goto('/console/easy/settings')
  await expect(page.getByTestId('easy-settings-engine')).toHaveText('The team works with Claude Code.')
  await expect(page.getByTestId('easy-settings-choice')).toHaveCount(3)
  await expect(page.getByTestId('easy-settings-ai-save')).toBeDisabled()
  await page.locator('[data-testid="easy-settings-choice"][data-tier="heavy"]').click()
  await page.getByTestId('easy-settings-ai-save').click()
  await expect.poll(() => saved).toEqual({ llm_default_tier: 'heavy' })
  await expect(page.getByTestId('easy-settings-ai')).toContainText('It takes effect after TARS restarts')

  const bar = page.getByTestId('easy-settings-restart')
  await expect(bar).toBeVisible()
  await page.getByTestId('easy-settings-restart-button').click()
  await expect(page.getByTestId('easy-settings-restart-button')).toHaveText('Restart now?')
  expect(restarts, 'the first click only asks').toBe(0)
  await plainWords(page)
})

test('a console without the owner role is told why there is nothing to change', async ({ page }) => {
  await vibeOnly(page)
  await page.route('**/v1/admin/config/schema', (route) => route.fulfill({ status: 403, json: { error: 'admin role required' } }))
  await page.goto('/console/easy/settings')
  await expect(page.getByTestId('easy-settings-not-owner')).toContainText('Only the owner of this computer')
  await expect(page.getByTestId('phones-card')).toHaveCount(0)
})

test('the header has no empty box in vibe mode', async ({ page }) => {
  await vibeOnly(page)
  await page.goto('/console/easy/settings')
  await expect(page.getByTestId('easy-settings')).toBeVisible()
  for (const chip of await page.locator('.auth-chip').all()) {
    expect((await chip.innerText()).trim(), 'an auth chip in vibe mode holds Sign out').not.toBe('')
  }
})
