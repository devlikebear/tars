// CASE's eight expressions (#1190): eyes, mouth, eyebrows and antenna
// change shape per expression, not just the glow colour. This spec builds
// a side-by-side gallery of all eight purely in the page's DOM — cloning
// the real `.companion-body` and relabelling its `expr-<name>` class — so
// there is no debug prop, query param or extra markup shipped in the
// production bundle. It then screenshots the gallery (for a human to look
// at; the PNG itself is not committed, it lives under test-results/) and
// asserts every expression renders as a different image from every other.

import { expect, test } from '@playwright/test'

const EXPRESSIONS = ['neutral', 'working', 'alert', 'happy', 'upset', 'wary', 'sleepy', 'greeting'] as const

// Reduced motion stops the float/blink animations, so the gallery (and the
// screenshot) shows each expression by shape alone, the same way a user
// with that setting sees it.
test.use({ contextOptions: { reducedMotion: 'reduce' } })

test("CASE's eight expressions are shapes, not a glow colour, and read apart from each other", async ({ page }, testInfo) => {
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')
  await expect(body).toBeVisible()
  // The default, idle state (no activity, no failures) is neutral — the
  // one shape this test clones and relabels into the other seven. Every
  // mount greets first (#1191's `justArrived` cue) for a few seconds; this
  // retries (10s default) past that before cloning.
  await expect(body).toHaveClass(/expr-neutral/)

  await page.evaluate((expressions) => {
    const original = document.querySelector('.companion-pet .companion-body')
    if (!original) throw new Error('companion body not found')
    const gallery = document.createElement('div')
    gallery.id = 'companion-expression-gallery'
    gallery.style.position = 'fixed'
    gallery.style.left = '0'
    gallery.style.top = '0'
    gallery.style.zIndex = '9999'
    gallery.style.display = 'flex'
    gallery.style.gap = '16px'
    gallery.style.padding = '12px'
    gallery.style.background = 'var(--surface-elevated)'
    for (const name of expressions) {
      const clone = original.cloneNode(true) as HTMLElement
      clone.className = clone.className.replace(/expr-\S+/, `expr-${name}`)
      clone.setAttribute('data-expr-preview', name)
      const cell = document.createElement('div')
      cell.style.position = 'relative'
      cell.style.width = '74px'
      cell.style.height = '86px'
      cell.style.display = 'grid'
      cell.style.placeItems = 'end center'
      cell.appendChild(clone)
      gallery.appendChild(cell)
    }
    document.body.appendChild(gallery)
  }, EXPRESSIONS)

  const gallery = page.locator('#companion-expression-gallery')
  await expect(gallery).toBeVisible()
  await gallery.screenshot({ path: testInfo.outputPath('companion-expressions.png') })

  const shots = new Map<string, Buffer>()
  for (const name of EXPRESSIONS) {
    shots.set(name, await page.locator(`[data-expr-preview="${name}"]`).screenshot())
  }
  const names = [...shots.keys()]
  for (let i = 0; i < names.length; i++) {
    for (let j = i + 1; j < names.length; j++) {
      const a = shots.get(names[i])
      const b = shots.get(names[j])
      expect(a?.equals(b ?? Buffer.alloc(0)), `expr-${names[i]} and expr-${names[j]} render identically`).toBe(false)
    }
  }
})
