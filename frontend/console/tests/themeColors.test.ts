import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'

import { mermaidThemeVariables, terminalSearchDecorations, terminalTheme, themeColors } from '../src/lib/themeColors.ts'

const appCss = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8')

function token(name: string): string {
  const match = new RegExp(`--${name}:\\s*([^;]+);`).exec(appCss)
  assert.ok(match, `app.css defines --${name}`)
  return match[1].trim().toLowerCase()
}

test('literal theme colors match the app.css tokens', () => {
  const pairs: [keyof typeof themeColors, string][] = [
    ['surfaceBase', 'surface-base'],
    ['surface', 'surface'],
    ['surfaceElevated', 'surface-elevated'],
    ['surfaceHover', 'surface-hover'],
    ['surfaceInset', 'surface-inset'],
    ['borderDefault', 'border-default'],
    ['borderStrong', 'border-strong'],
    ['textPrimary', 'text-primary'],
    ['textSecondary', 'text-secondary'],
    ['textTertiary', 'text-tertiary'],
    ['primary', 'primary'],
    ['primaryHover', 'primary-hover'],
    ['primaryContrast', 'primary-contrast'],
  ]
  for (const [key, name] of pairs) assert.equal(themeColors[key], token(name), key)
})

test('--primary-rgb is the same color as --primary', () => {
  const hex = token('primary').slice(1)
  const rgb = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16)).join(', ')
  assert.equal(token('primary-rgb'), rgb)
})

test('canvas and mermaid colors are plain hex, never CSS variables', () => {
  for (const [name, value] of Object.entries({ ...terminalTheme, ...terminalSearchDecorations, ...mermaidThemeVariables })) {
    if (typeof value !== 'string') continue
    assert.match(value, /^#[0-9a-f]{6}$/i, `${name} must be #RRGGBB`)
  }
})

// The old amber accent must not creep back in as a literal.
test('no component hard-codes the retired amber accent', () => {
  const root = new URL('../src/', import.meta.url)
  const files = readdirSync(root, { recursive: true, encoding: 'utf8' }).filter((f) => /\.(svelte|css|ts)$/.test(f))
  for (const file of files) {
    const source = readFileSync(new URL(file, root), 'utf8')
    assert.doesNotMatch(source, /#e09145|224,\s*145,\s*69/i, file)
  }
})
