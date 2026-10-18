// CASE's figure (case-companion phase 1.5): a codex-drawn robot body from
// one sprite sheet, with the face drawn in CSS on its screen. The sheet's
// layout comes from scripts/case_sprites.py (caseSprite.generated.ts); this
// turns it into the custom properties CaseFigure.svelte's CSS reads, so a
// regenerated sheet needs no hand-edited pixel values.
//
// Which pose frame shows is decided in CSS from the `expr-*` class alone
// (CaseFigure.svelte), not here: e2e/companion-expressions.spec.ts clones
// `.companion-body` and swaps only that class, and every expression must
// still draw its own pose.

export interface CaseSprite {
  readonly cell: number
  readonly frames: readonly string[]
  readonly screen: readonly [number, number, number, number] | readonly number[]
  readonly led: Readonly<Record<string, readonly number[]>>
}

/** The display size of the figure in CSS pixels (the cell is drawn at 2x). */
export const CASE_FIGURE_SIZE = 64

function px(value: number): string {
  // Two decimals is well under a device pixel at any zoom; it keeps the
  // style string stable and short.
  return `${Math.round(value * 100) / 100}px`
}

/**
 * The inline custom properties for `.companion-body`: the sheet URL and
 * size, each frame's x offset (`--case-frame-<name>`), the screen
 * rectangle, and each frame's LED centre and radius, all scaled from
 * sprite cells to `size` CSS pixels.
 */
export function caseFigureStyle(sheetUrl: string, sprite: CaseSprite, size = CASE_FIGURE_SIZE): string {
  const k = size / sprite.cell
  const vars: string[] = [
    `--case-sheet: url("${sheetUrl}")`,
    `--case-sheet-width: ${px(size * sprite.frames.length)}`,
  ]
  sprite.frames.forEach((name, i) => {
    vars.push(`--case-frame-${name}: ${px(-size * i)}`)
  })
  const [sx, sy, sw, sh] = sprite.screen
  vars.push(`--case-screen-x: ${px(sx * k)}`, `--case-screen-y: ${px(sy * k)}`)
  vars.push(`--case-screen-w: ${px(sw * k)}`, `--case-screen-h: ${px(sh * k)}`)
  for (const name of sprite.frames) {
    const led = sprite.led[name]
    if (!led) continue
    const [cx, cy, r] = led
    vars.push(`--case-led-${name}-x: ${px(cx * k)}`, `--case-led-${name}-y: ${px(cy * k)}`, `--case-led-${name}-r: ${px(r * k)}`)
  }
  return vars.join('; ')
}
