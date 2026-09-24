// Korean console (#968). With a Korean browser, the chat workbench and each
// dock panel render the console's own words in Korean. Server, LLM, and user
// content may still be English, so this reads only the chrome: buttons,
// headings, labels, options, and title/aria-label/placeholder attributes.

import { expect, test, type Locator, type Page } from '@playwright/test'

test.use({ locale: 'ko-KR' })

// Names the Korean UI keeps in English on purpose (see src/i18n/ko.ts).
const keptInEnglish = [
  'TARS', 'Git', 'MCP', 'Pulse', 'LLM', 'cwd', 'JSON', 'YAML',
  'Light', 'Standard', 'Heavy', 'Ctrl', 'Cmd', 'Alt', 'Shift', 'Enter', 'Esc',
]

// Two English words in a row is a phrase that escaped translation; a lone
// product name, key, or identifier is not.
const englishRun = /[A-Za-z]{2,}[ \t]+[A-Za-z]{2,}/

function untranslated(text: string): boolean {
  let rest = text
  for (const name of keptInEnglish) rest = rest.replaceAll(name, ' ')
  return englishRun.test(rest)
}

async function chromeTexts(scope: Locator): Promise<string[]> {
  return scope.evaluate((root) => {
    const seen = new Set<string>()
    const add = (value: string | null | undefined) => {
      const text = value?.replace(/\s+/g, ' ').trim()
      if (text) seen.add(text)
    }
    // Content, not chrome: terminal output, rendered markdown and code, chat
    // messages, session titles (in the sidebar and the palette), and the
    // server's tool and skill lists in the session config panel.
    const content = '.xterm, .markdown-body, pre, code, .chat-msg, .session-title, .session-item, .config-list, [data-group="session"]'
    // An element's own words: drop nested content and <select>s, whose
    // options (tier and model names, for one) are read one by one instead.
    const ownText = (el: Element) => {
      const copy = el.cloneNode(true) as Element
      copy.querySelectorAll(`${content}, select`).forEach((node) => node.remove())
      return copy.textContent
    }
    root.querySelectorAll('button, label, h1, h2, h3, h4, th, legend, summary, option, [role="option"]').forEach((el) => {
      if (!el.closest(content)) add(ownText(el))
    })
    root.querySelectorAll('[title], [aria-label], [placeholder]').forEach((el) => {
      if (el.closest(content)) return
      add(el.getAttribute('title'))
      add(el.getAttribute('aria-label'))
      add(el.getAttribute('placeholder'))
    })
    return [...seen]
  })
}

async function newSession(page: Page) {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await expect(page.locator('.chat-rail')).toBeVisible()
}

test('the chat workbench chrome is Korean', async ({ page }) => {
  await newSession(page)
  const texts = [
    ...(await chromeTexts(page.locator('.dock-left'))),
    ...(await chromeTexts(page.locator('.chat-main'))),
    ...(await chromeTexts(page.locator('.chat-rail'))),
  ]
  expect(texts.length).toBeGreaterThan(5)
  expect(texts.filter(untranslated)).toEqual([])
})

const dockPanels = ['artifacts', 'git', 'tasks', 'context', 'prior', 'prompt', 'config', 'skillExtraction', 'cron', 'health'] as const

test('every dock panel is Korean', async ({ page }) => {
  await newSession(page)
  const found: string[] = []
  for (const id of dockPanels) {
    const toggle = page.locator(`.chat-rail [data-panel="${id}"]`)
    await toggle.click()
    const pane = page.locator('.dock-right')
    await expect(pane).toBeVisible()
    // Panels load their data after opening; read them once that settles.
    await expect(pane.getByText(/불러오는 중|Loading/)).toHaveCount(0)
    for (const text of (await chromeTexts(pane)).filter(untranslated)) found.push(`${id}: ${text}`)
    await toggle.click()
    await expect(pane).toHaveCount(0)
  }
  expect(found).toEqual([])
})

test('the palette and shortcut help are Korean', async ({ page }) => {
  await newSession(page)
  await page.locator('.chat-main textarea').click()
  await page.keyboard.press('Control+K')
  const palette = page.getByRole('dialog')
  await expect(palette).toBeVisible()
  const found = (await chromeTexts(palette)).filter(untranslated)
  await page.keyboard.press('Escape')

  // `?` opens help only when focus is outside text inputs.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await page.keyboard.press('Shift+Slash')
  const help = page.getByRole('dialog')
  await expect(help).toBeVisible()
  found.push(...(await chromeTexts(help)).filter(untranslated))
  expect(found).toEqual([])
})
