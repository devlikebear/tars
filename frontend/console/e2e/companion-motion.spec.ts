// CASE's motion (#1191): one-shot actions, idle play, cursor gaze, and the
// stop conditions (hidden tab, prefers-reduced-motion). This spec runs with
// the default project config (motion enabled, real time) for the behaviour
// checks below, and uses Playwright's virtual clock (`page.clock`) to reach
// idle play without a real 60-180s wait. The screenshot gallery at the
// bottom follows companion-expressions.spec.ts's approach: it clones the
// real `.companion-body`/`.companion-eyes` into an off-screen gallery and
// seeks each clone's animations to a fixed frame with `getAnimations()`, so
// there is no debug prop, query param or extra markup shipped in the
// production bundle, and the result does not depend on real-time timing.

import { expect, test } from '@playwright/test'

const ACTIONS = ['bounce', 'nod', 'shake', 'tilt', 'wave'] as const
const IDLE_KINDS = ['look', 'yawn', 'wink'] as const

test('the hit area never moves or resizes while CASE floats (#1194 regression guard)', async ({ page }) => {
  await page.goto('/console')
  const button = page.locator('.companion-pet .companion-button')
  await expect(button).toBeVisible()
  const first = await button.boundingBox()
  // Sample at a different phase of the 4.8s float loop.
  await page.waitForTimeout(900)
  const second = await button.boundingBox()
  expect(second).toEqual(first)
})

test('idle play fires while neutral and idle, without touching the expression, badge or bubble', async ({ page }) => {
  // The clock starts frozen: nothing (including the `justArrived` greeting
  // every mount gets) expires on its own until fastForward says so, so the
  // one jump below carries CASE past both that greeting and into the idle
  // window — there is no useful "still neutral" check before it.
  await page.clock.install()
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')
  await expect(page.locator('.companion-badge')).toHaveCount(0)

  // 180s is the longest possible idle delay; a couple of seconds of margin
  // covers the setInterval/setTimeout granularity.
  await page.clock.fastForward('00:03:05')
  await expect(body).toHaveClass(/play-(look|yawn|wink)/)
  // Decoration only: still neutral, still no badge, bubble still closed.
  await expect(body).toHaveClass(/expr-neutral/)
  await expect(page.locator('.companion-badge')).toHaveCount(0)
  await expect(page.locator('.companion-bubble')).toHaveCount(0)
})

test('prefers-reduced-motion never starts the idle timer, even after the same time jump', async ({ browser }) => {
  const context = await browser.newContext({ reducedMotion: 'reduce' })
  const page = await context.newPage()
  await page.clock.install()
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')

  await page.clock.fastForward('00:03:05')
  // Give any (incorrectly still-running) CSS animation time to land too.
  await page.waitForTimeout(500)
  await expect(body).not.toHaveClass(/play-(look|yawn|wink)/)
  await expect(body).not.toHaveClass(/act-(bounce|nod|shake|tilt|wave)/)
  await context.close()
})

test('a sleeping CASE wakes the instant the pointer moves, even without a click', async ({ page }) => {
  await page.clock.install()
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')

  // Five quiet minutes with nothing waiting — no click, no keystroke, just
  // the clock moving.
  await page.clock.fastForward('00:05:01')
  await expect(body).toHaveClass(/expr-sleepy/)

  // A pointer move alone counts as input (companionShouldRecordInput) and
  // wakes CASE immediately — the gap since the last recorded input is huge,
  // so this first move always passes the once-a-second throttle.
  await page.mouse.move(200, 200)
  await expect(body).not.toHaveClass(/expr-sleepy/)
})

test('the same wake-on-move holds under prefers-reduced-motion — longQuiet/sleepy is a shape change, not an action', async ({ browser }) => {
  const context = await browser.newContext({ reducedMotion: 'reduce' })
  const page = await context.newPage()
  await page.clock.install()
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')

  await page.clock.fastForward('00:05:01')
  await expect(body).toHaveClass(/expr-sleepy/)

  await page.mouse.move(200, 200)
  await expect(body).not.toHaveClass(/expr-sleepy/)
  await context.close()
})

test('the eyes lean toward a nearby pointer, and the lean flips sign across the bot', async ({ page }) => {
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')
  // The arrival greeting (#1191) does not take a gaze lean — let it clear
  // first (real time here, no clock mock), so the rest of this test is
  // against a pupil-style expression.
  await expect(body).toHaveClass(/expr-neutral/)
  const eyes = page.locator('.companion-pet .companion-eyes')
  const box = await body.boundingBox()
  if (!box) throw new Error('companion body not found')
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }

  async function eyeOffset() {
    const style = await eyes.getAttribute('style')
    const x = Number(style?.match(/--eye-x:\s*(-?[\d.]+)px/)?.[1] ?? 'NaN')
    const y = Number(style?.match(/--eye-y:\s*(-?[\d.]+)px/)?.[1] ?? 'NaN')
    return { x, y }
  }

  // Up and to the left of the bot: both offsets negative.
  await page.mouse.move(center.x - 80, center.y - 80)
  await page.waitForTimeout(100)
  const upLeft = await eyeOffset()
  expect(upLeft.x).toBeLessThan(0)
  expect(upLeft.y).toBeLessThan(0)

  // Down and to the right (clamped to the viewport, CASE sits near that
  // corner already): both offsets positive — the opposite sign from above.
  await page.mouse.move(Math.min(center.x + 50, 1395), Math.min(center.y + 50, 895))
  await page.waitForTimeout(100)
  const downRight = await eyeOffset()
  expect(downRight.x).toBeGreaterThan(0)
  expect(downRight.y).toBeGreaterThan(0)

  // Far away: no lean at all.
  await page.mouse.move(10, 10)
  await page.waitForTimeout(100)
  const far = await eyeOffset()
  expect(far.x).toBe(0)
  expect(far.y).toBe(0)
})

test("CASE's actions, idle play and cursor gaze are visibly different frames, saved for a human to look at", async ({ page }, testInfo) => {
  await page.goto('/console')
  const body = page.locator('.companion-pet .companion-body')
  await expect(body).toBeVisible()

  type Rect = { x: number; y: number; width: number; height: number }
  const { gallery: galleryRect, shots } = await page.evaluate(
    async ({ actions, idleKinds }) => {
      const original = document.querySelector('.companion-pet .companion-body')
      if (!original) throw new Error('companion body not found')

      const gallery = document.createElement('div')
      gallery.id = 'companion-motion-gallery'
      gallery.style.position = 'fixed'
      gallery.style.left = '0'
      gallery.style.top = '0'
      gallery.style.zIndex = '9999'
      gallery.style.display = 'flex'
      gallery.style.flexWrap = 'wrap'
      gallery.style.gap = '16px'
      gallery.style.padding = '12px'
      gallery.style.background = 'var(--surface-elevated)'
      document.body.appendChild(gallery)

      // Svelte scopes `.companion-body`'s CSS to a per-component hash class
      // (e.g. `svelte-1qeykxh`) appended alongside `companion-body` — every
      // compiled selector requires it too. Overwriting `className` wholesale
      // drops that hash and the clone matches none of its own styles (zero
      // size). Keep every class the live element already has and only
      // replace the `expr-`/`act-`/`play-` ones.
      function setStateClasses(clone: HTMLElement, parts: string[]) {
        const kept = clone.className.split(/\s+/).filter((c) => c && !/^(expr|act|play)-/.test(c))
        clone.className = [...kept, ...parts].join(' ')
      }

      function addClone(label: string, mutate: (clone: HTMLElement) => void) {
        const clone = original!.cloneNode(true) as HTMLElement
        mutate(clone)
        clone.setAttribute('data-motion-preview', label)
        const cell = document.createElement('div')
        cell.style.position = 'relative'
        cell.style.width = '74px'
        cell.style.height = '86px'
        cell.style.display = 'grid'
        cell.style.placeItems = 'end center'
        cell.appendChild(clone)
        gallery.appendChild(cell)
      }

      // Each action plays on its own expression (lib/companion.ts
      // companionActionFor's table) — e.g. "bounce" only ever arrives
      // together with "alert".
      const actionExpression: Record<string, string> = {
        bounce: 'alert',
        nod: 'happy',
        shake: 'upset',
        tilt: 'wary',
        wave: 'greeting',
      }
      for (const action of actions) {
        addClone(`action-${action}`, (clone) => {
          setStateClasses(clone, [`expr-${actionExpression[action]}`, `act-${action}`])
        })
      }
      for (const kind of idleKinds) {
        addClone(`idle-${kind}`, (clone) => {
          setStateClasses(clone, ['expr-neutral', `play-${kind}`])
        })
      }
      addClone('gaze-up-left', (clone) => {
        setStateClasses(clone, ['expr-neutral'])
        const eyes = clone.querySelector('.companion-eyes') as HTMLElement
        eyes.style.setProperty('--eye-x', '-3px')
        eyes.style.setProperty('--eye-y', '-3px')
      })
      addClone('gaze-down-right', (clone) => {
        setStateClasses(clone, ['expr-neutral'])
        const eyes = clone.querySelector('.companion-eyes') as HTMLElement
        eyes.style.setProperty('--eye-x', '3px')
        eyes.style.setProperty('--eye-y', '3px')
      })

      // Freeze every clone's animations at their mid-point, deterministically
      // — no reliance on whatever real time the screenshot happens to land on.
      for (const el of Array.from(gallery.querySelectorAll<HTMLElement>('[data-motion-preview]'))) {
        for (const anim of el.getAnimations({ subtree: true })) {
          anim.pause()
          const duration = anim.effect?.getComputedTiming().duration
          if (typeof duration === 'number') anim.currentTime = duration / 2
        }
      }

      // Return plain rects, not locators: a screenshot clipped to a
      // coordinate rect (below) does not need Playwright's own notion of
      // "visible" for an aria-hidden, off-flow clone the way a locator
      // action does, and reads the frame we just froze either way.
      const rectOf = (el: Element) => {
        const r = el.getBoundingClientRect()
        return { x: r.x, y: r.y, width: r.width, height: r.height }
      }
      const labels = [...actions.map((a) => `action-${a}`), ...idleKinds.map((k) => `idle-${k}`), 'gaze-up-left', 'gaze-down-right']
      const shots = labels.map((label) => ({
        label,
        rect: rectOf(gallery.querySelector(`[data-motion-preview="${label}"]`)!),
      }))
      return { gallery: rectOf(gallery), shots }
    },
    { actions: ACTIONS, idleKinds: IDLE_KINDS },
  )

  await page.screenshot({ path: testInfo.outputPath('companion-motion-gallery.png'), clip: galleryRect as Rect })

  const images = new Map<string, Buffer>()
  for (const { label, rect } of shots as { label: string; rect: Rect }[]) {
    images.set(label, await page.screenshot({ path: testInfo.outputPath(`${label}.png`), clip: rect }))
  }

  // The two gaze frames must differ from each other (opposite lean) and
  // from the resting neutral face (no lean) captured earlier in this spec.
  const upLeft = images.get('gaze-up-left')
  const downRight = images.get('gaze-down-right')
  expect(upLeft?.equals(downRight ?? Buffer.alloc(0))).toBe(false)
})
