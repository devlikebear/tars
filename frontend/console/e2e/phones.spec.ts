// Pairing a phone from the settings page (docs/decisions/mobile-remote.md
// §4). The runner has no Tailscale, so the remote-access status is the one
// response mocked here; the pairing code, the trade at /v1/auth/device-pair,
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
