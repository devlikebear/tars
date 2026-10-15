// tars#1257: when the server starts serving a different console bundle
// than the one running in this tab, the console shows a small,
// dismissible "TARS was updated — Reload" notice on the next stream
// reconnect or tab re-show — never automatically. A real server restart
// can't be driven deterministically from a spec (and risks the #1194
// class of bug: a shared e2e server mid-restart breaks every other
// running spec), so this uses the `-tags e2e`-only POST
// /v1/e2e/console-build (internal/tarsserver/e2e_console_build.go) to
// override what healthz reports, then simulates the tab becoming visible
// again to trigger the check — the same code path a real tab switch
// takes (App.svelte's visibilitychange listener).

import { expect, test, type Page } from '@playwright/test'

test.use({ contextOptions: { reducedMotion: 'reduce' } })

const banner = (page: Page) => page.locator('.console-update-banner')
const reloadButton = (page: Page) => page.locator('.console-update-reload')
const dismissButton = (page: Page) => page.locator('.console-update-dismiss')

async function getConsoleBuild(page: Page): Promise<string> {
  const response = await page.request.get('/v1/healthz')
  const body = (await response.json()) as { console_build?: string }
  return body.console_build ?? ''
}

async function setConsoleBuild(page: Page, build: string) {
  const response = await page.request.post('/v1/e2e/console-build', { data: { build } })
  expect(response.ok()).toBeTruthy()
}

// App.svelte's listener only checks console_build on a *real*
// hidden->visible transition (like lib/presence.ts's hiddenAt tracking,
// #1249) — a visibilitychange event that fires while the tab was already
// visible does nothing, by design (a multi-page browser context can emit
// one of those without an actual transition). So faking a re-show means
// overriding document.visibilityState to 'hidden' and dispatching first,
// then overriding it back to 'visible' and dispatching again — document
// .visibilityState is normally read-only, hence the property override.
async function simulateTabReshow(page: Page) {
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

test('the update notice appears after a changed build, survives a dismiss+newer-build cycle, and reloads on click', async ({ page }, testInfo) => {
  await page.goto('/console')
  await expect(banner(page)).toHaveCount(0)

  const originalBuild = await getConsoleBuild(page)
  // Restore the real server's own value when this test ends, whatever
  // happens — other specs reuse this same server process.
  try {
    // Same build: re-showing the tab must not invent a notice out of
    // nothing.
    await simulateTabReshow(page)
    await page.waitForTimeout(200)
    await expect(banner(page)).toHaveCount(0)

    // A genuinely different build: the next tab re-show shows the notice.
    await setConsoleBuild(page, 'e2e-build-one')
    await simulateTabReshow(page)
    await expect(banner(page)).toBeVisible()
    await expect(reloadButton(page)).toHaveText(/TARS was updated|TARS가 업데이트됐어요/)

    await page.screenshot({ path: testInfo.outputPath('console-update-notice.png') })

    // Dismissing hides it, and re-showing the tab again for the *same*
    // build must not bring it back.
    await dismissButton(page).click()
    await expect(banner(page)).toHaveCount(0)
    await simulateTabReshow(page)
    await page.waitForTimeout(200)
    await expect(banner(page)).toHaveCount(0)

    // A newer build than the one dismissed shows again.
    await setConsoleBuild(page, 'e2e-build-two')
    await simulateTabReshow(page)
    await expect(banner(page)).toBeVisible()

    // Clicking the message reloads the page — never automatic.
    await page.evaluate(() => {
      ;(window as unknown as { __e2eBeforeReload?: boolean }).__e2eBeforeReload = true
    })
    await Promise.all([page.waitForNavigation(), reloadButton(page).click()])
    const marker = await page.evaluate(() => (window as unknown as { __e2eBeforeReload?: boolean }).__e2eBeforeReload)
    expect(marker).toBeUndefined()
  } finally {
    await setConsoleBuild(page, originalBuild)
  }
})
