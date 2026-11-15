// A pipeline's page on an iPhone offers to open it in the TARS app
// (docs/decisions/mobile-remote.md §7): a Telegram alert links to this page,
// and the offer is the step into the app. The app pairs only over https, so
// the page is opened at a stand-in https address whose every request is
// passed to the real e2e server.
import { expect, test, type Page } from '@playwright/test'

const origin = 'https://tars-e2e.test'
const iPhoneUA = 'Mozilla/5.0 (iPhone; CPU iPhone OS 26_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Mobile/15E148 Safari/604.1'

async function overHTTPS(page: Page, server: string) {
  await page.route(`${origin}/**`, async (route) => {
    const url = new URL(route.request().url())
    // Streams would hold the proxy open; the page works without them.
    if (url.pathname.endsWith('/stream')) return route.abort()
    const response = await route.fetch({ url: `${server}${url.pathname}${url.search}` })
    await route.fulfill({ response })
  })
}

test.describe('on an iPhone', () => {
  test.use({ userAgent: iPhoneUA, viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true })

  test('a pipeline page offers the app, and closing the offer keeps the browser', async ({ page, baseURL }) => {
    await overHTTPS(page, baseURL ?? '')
    await page.goto(`${origin}/console/focus/e2e-handoff`)
    const offer = page.getByTestId('app-handoff')
    await expect(offer.getByRole('link', { name: 'Open in app' })).toHaveAttribute(
      'href',
      'tars-mobile://focus?url=https%3A%2F%2Ftars-e2e.test&session=e2e-handoff',
    )
    await offer.getByRole('button', { name: 'Stay in the browser' }).click()
    await expect(offer).toHaveCount(0)
  })

  test('other pages offer nothing', async ({ page, baseURL }) => {
    await overHTTPS(page, baseURL ?? '')
    await page.goto(`${origin}/console/focus`)
    await expect(page.locator('.shell')).toBeVisible()
    await expect(page.getByTestId('app-handoff')).toHaveCount(0)
  })
})

test('a desktop browser is not offered the app', async ({ page, baseURL }) => {
  await overHTTPS(page, baseURL ?? '')
  await page.goto(`${origin}/console/focus/e2e-handoff`)
  await expect(page.locator('.shell')).toBeVisible()
  await expect(page.getByTestId('app-handoff')).toHaveCount(0)
})
