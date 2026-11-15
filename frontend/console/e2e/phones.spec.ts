// Pairing a phone from the settings page (docs/decisions/mobile-remote.md
// §4). The remote-access status is the one response mocked here (the runner
// may or may not have Tailscale); the pairing code, the trade at /v1/auth/device-pair,
// the device list, revoking and the folder list are the real server's.
import { expect, test, type Page } from '@playwright/test'

const servedStatus = {
  desired_enabled: true,
  desired_https_port: 443,
  target_url: 'http://127.0.0.1:43180',
  url: 'https://mac-mini.tail1.ts.net',
  status: { installed: true, logged_in: true, host_name: 'mac-mini', tailnet_url: 'mac-mini.tail1.ts.net', serve_active: true, serve_port: 443, owned_by_tars: true },
  checks: [],
}

async function openPhones(page: Page) {
  await page.goto('/console/config')
  await page.locator('.config-tabs').getByRole('button').nth(4).click()
  const card = page.getByTestId('phones-card')
  await expect(card).toBeVisible()
  return card
}

test('without a Tailscale address, a phone cannot be paired', async ({ page }) => {
  // Mocked too: a developer's Mac may really be serving TARS over Tailscale.
  const notServing = { ...servedStatus, desired_enabled: false, url: '', status: { ...servedStatus.status, serve_active: false, owned_by_tars: false } }
  await page.route('**/v1/admin/remote-access/status', (route) => route.fulfill({ json: notServing }))
  const card = await openPhones(page)
  await expect(card.getByRole('status')).toContainText('Turn on Tailscale remote access')
  await expect(card.getByTestId('phones-pair')).toBeDisabled()
})

test('pair a phone with the QR code, then revoke it', async ({ page, request }) => {
  await page.route('**/v1/admin/remote-access/status', (route) => route.fulfill({ json: servedStatus }))
  const card = await openPhones(page)

  await card.getByTestId('phones-pair').click()
  await expect(card.getByRole('img', { name: 'Pairing QR code' })).toBeVisible()
  await expect(card.getByTestId('phones-pairing')).toContainText('https://mac-mini.tail1.ts.net')
  await expect(card.getByTestId('phones-pairing')).toContainText('Expires in')
  const code = (await card.getByTestId('phones-code').textContent())?.trim() ?? ''
  expect(code).not.toBe('')

  // The phone trades the code; the card sees it and closes the code.
  const paired = await request.post('/v1/auth/device-pair', { data: { code, name: 'E2E iPhone' } })
  expect(paired.ok()).toBeTruthy()
  await expect(card.getByTestId('phones-pairing')).toHaveCount(0)
  const row = card.getByTestId('phones-device').filter({ hasText: 'E2E iPhone' })
  await expect(row).toContainText('Paired')

  page.once('dialog', (dialog) => dialog.accept())
  await row.getByRole('button', { name: 'Revoke' }).click()
  await expect(row).toContainText('Revoked')
  await expect(row.getByRole('button', { name: 'Revoke' })).toHaveCount(0)
})

test('registered folders show and come off the list', async ({ page, request }) => {
  const workspace = process.env.TARS_E2E_WORKSPACE ?? ''
  const missing = `${workspace}/no-such-folder`
  const put = await request.put('/v1/admin/devices/folders', { data: { folders: [workspace, missing] } })
  expect(put.ok()).toBeTruthy()

  const card = await openPhones(page)
  const rows = card.getByTestId('phones-folder')
  await expect(rows).toHaveCount(2)
  await expect(rows.filter({ hasText: 'no-such-folder' })).toContainText('Not found')

  await rows.filter({ hasText: 'no-such-folder' }).getByRole('button', { name: 'Remove' }).click()
  await expect(rows).toHaveCount(1)
  const after = await (await request.get('/v1/admin/devices/folders')).json()
  expect(after.folders.map((f: { path: string }) => f.path)).toEqual([workspace])

  await request.put('/v1/admin/devices/folders', { data: { folders: [] } })
})

// Keeping the computer awake for a paired phone (desk_keep_awake.go). The
// status is mocked for the screen: whether it holds depends on the host
// (macOS only) and on phones other specs pair. The choice itself goes to the
// real server.
test('keeping the computer awake: the toggle saves the choice and says what holds', async ({ page }) => {
  let saved: unknown = null
  await page.route('**/v1/admin/devices/keep-awake', (route) => {
    if (route.request().method() === 'PUT') {
      saved = route.request().postDataJSON()
      return route.fulfill({ json: { enabled: true, supported: true, active: false, reason: 'no_phone' } })
    }
    return route.fulfill({ json: { enabled: false, supported: true, active: false } })
  })
  const card = await openPhones(page)
  const section = card.getByTestId('phones-keep-awake')
  const toggle = section.getByRole('checkbox', { name: 'Stay awake while plugged in and a phone is paired' })
  await expect(toggle).not.toBeChecked()
  await expect(section.getByTestId('phones-keep-awake-note')).toHaveCount(0)
  await toggle.check()
  await expect(section.getByTestId('phones-keep-awake-note')).toHaveText('On, but no phone is paired, so it may sleep.')
  expect(saved).toEqual({ enabled: true })
})

test('a system that cannot stay awake says so and offers nothing to turn on', async ({ page }) => {
  await page.route('**/v1/admin/devices/keep-awake', (route) =>
    route.fulfill({ json: { enabled: false, supported: false, active: false } }),
  )
  const section = (await openPhones(page)).getByTestId('phones-keep-awake')
  await expect(section.getByRole('checkbox')).toBeDisabled()
  await expect(section.getByTestId('phones-keep-awake-note')).toHaveText('Not available on this system yet: macOS only.')
})

test('the keep-awake choice is stored by the server', async ({ request }) => {
  const on = await request.put('/v1/admin/devices/keep-awake', { data: { enabled: true } })
  expect(on.ok()).toBeTruthy()
  expect((await on.json()).enabled).toBe(true)
  expect((await (await request.get('/v1/admin/devices/keep-awake')).json()).enabled).toBe(true)
  expect((await request.put('/v1/admin/devices/keep-awake', { data: {} })).status()).toBe(400)
  const off = await (await request.put('/v1/admin/devices/keep-awake', { data: { enabled: false } })).json()
  expect(off).toMatchObject({ enabled: false, active: false })
})
