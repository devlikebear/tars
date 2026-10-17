// Korean settings page (#1241). With a Korean browser, every tab, the
// cross-tab search, and the Tailscale remote-access card render the
// console's own words in Korean — YAML paths, raw values and option enums
// stay in English on purpose. Read-only: this spec never saves a change, so
// it is safe to run alongside other specs touching the same server.
import { expect, test, type Locator, type Page } from '@playwright/test'

test.use({ locale: 'ko-KR' })

// Names the Korean UI keeps in English on purpose (see
// src/i18n/sections/config.ts and src/i18n/sections/remoteAccess.ts).
const keptInEnglish = [
  'TARS', 'Git', 'MCP', 'Pulse', 'LLM', 'cwd', 'JSON', 'YAML', 'API', 'URL', 'HTTPS',
  'Light', 'Standard', 'Heavy', 'Ctrl', 'Cmd', 'Alt', 'Shift', 'Enter', 'Esc',
  'Tailscale', 'Perplexity', 'Telegram', 'Initiative', 'Embodiment', 'OAuth', 'CASE',
]

const englishRun = /[A-Za-z]{2,}[ \t]+[A-Za-z]{2,}/

function untranslated(text: string): boolean {
  let rest = text
  for (const name of keptInEnglish) rest = rest.replaceAll(name, ' ')
  return englishRun.test(rest)
}

// Reads every element's "own" chrome text (labels, descriptions, buttons,
// headings) under `scope`, excluding YAML paths, raw/diff values,
// search-result value previews, the raw check-code heading, and the
// Tailscale facts row (hostname/URL are real system data, not UI copy) —
// those stay in English on purpose. <select> is skipped: its <option>
// values are literal config enum values (e.g. "debug"), not translated
// labels.
async function settingsTexts(scope: Locator): Promise<string[]> {
  return scope.evaluate((root) => {
    const seen = new Set<string>()
    const add = (value: string | null | undefined) => {
      const text = value?.replace(/\s+/g, ' ').trim()
      if (text) seen.add(text)
    }
    const dataOnly = '.config-field-path, .config-search-hit-path, .config-search-hit-value, .diff-key, .diff-old, .diff-new, .remote-check-code, .remote-facts, code'
    const ownText = (el: Element) => {
      const copy = el.cloneNode(true) as Element
      copy.querySelectorAll(`${dataOnly}, select`).forEach((node) => node.remove())
      return copy.textContent
    }
    root.querySelectorAll('button, label, h1, h2, h3, h4, span, p, strong').forEach((el) => {
      if (el.closest(dataOnly) || el.querySelector(dataOnly)) return
      if (el.children.length === 0 || el.matches('span, p, strong')) add(ownText(el))
    })
    root.querySelectorAll('[title], [aria-label], [placeholder]').forEach((el) => {
      if (el.closest(dataOnly)) return
      add(el.getAttribute('title'))
      add(el.getAttribute('aria-label'))
      add(el.getAttribute('placeholder'))
    })
    return [...seen]
  })
}

// Reads one or more scopes separately, so a multi-match CSS selector (e.g.
// ".a, .b" matching two sibling cards) never trips evaluate()'s single-
// element requirement.
async function settingsTextsAll(scopes: Locator[]): Promise<string[]> {
  const all: string[] = []
  for (const scope of scopes) all.push(...(await settingsTexts(scope)))
  return all
}

async function gotoSettings(page: Page) {
  await page.goto('/console/config')
  await expect(page.locator('.config-tabs')).toBeVisible()
}

const tabIds = ['general', 'llm', 'chatTools', 'automation', 'channelsRemote', 'agents', 'extensions', 'advanced'] as const

test('every settings tab (basic fields, advanced expanded) is Korean', async ({ page }) => {
  await gotoSettings(page)
  const found: string[] = []
  for (const id of tabIds) {
    await page.locator('.config-tabs').getByRole('button').nth(tabIds.indexOf(id)).click()
    await expect(page.locator('.config-tab.active')).toHaveCount(1)
    const advancedToggle = page.locator('.config-advanced-toggle')
    if (await advancedToggle.count() > 0) await advancedToggle.click()
    const body = page.locator('.config-tab-body')
    for (const text of (await settingsTexts(body)).filter(untranslated)) found.push(`${id}: ${text}`)
    if (id === 'advanced') {
      const restartTexts = await settingsTextsAll([page.locator('.restart-section'), page.locator('.danger-zone')])
      for (const text of restartTexts.filter(untranslated)) found.push(`${id} (restart/danger): ${text}`)
    }
  }
  expect(found).toEqual([])
})

test('the Tailscale remote-access card is Korean', async ({ page }) => {
  await gotoSettings(page)
  await page.locator('.config-tabs').getByRole('button').nth(tabIds.indexOf('channelsRemote')).click()
  const card = page.locator('.remote-card')
  await expect(card).toBeVisible()
  await expect(card.locator('.remote-muted')).toHaveCount(0) // past the loading state
  const found = (await settingsTexts(card)).filter(untranslated)
  expect(found).toEqual([])
})

test('cross-tab search results and the search bar are Korean', async ({ page }) => {
  await gotoSettings(page)
  const search = page.locator('#config-search-input')
  await search.fill('session')
  const results = page.locator('.config-search-results')
  await expect(results).toBeVisible()
  const found = (await settingsTexts(page.locator('.config-search-bar'))).concat(await settingsTexts(results)).filter(untranslated)
  expect(found).toEqual([])
  await search.fill('')
})

test('at 640px, settings controls do not overflow horizontally', async ({ page }) => {
  await page.setViewportSize({ width: 640, height: 900 })
  await gotoSettings(page)
  await page.locator('.config-tabs').getByRole('button').nth(tabIds.indexOf('channelsRemote')).click()
  await expect(page.locator('.remote-card')).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(1)
})
