import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')

function cssRule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\n\\s*\\}`))
  assert.ok(match, `expected ${selector} rule`)
  return match[1]
}

test('JSON editor modal is centered in the content area beside the fixed sidebar', () => {
  const backdrop = cssRule('.modal-backdrop')
  const modal = cssRule('.json-editor-modal')

  assert.match(backdrop, /left:\s*var\(--nav-width\);/)
  assert.match(backdrop, /right:\s*0;/)
  assert.match(modal, /calc\(100vw - var\(--nav-width\) - 32px\)/)
})
