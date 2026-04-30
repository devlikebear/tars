import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')

test('Plugins sections show the deprecated Skills-first extension policy', () => {
  assert.match(source, /<span class="badge badge-warning" title="Plugins are deprecated/)
  assert.match(source, /Use Skills \(\.md \+ CLI\) for new extension work\./)
  assert.match(source, /Legacy plugin installs remain available/)
})

test('Plugins sections are collapsed by default as advanced legacy surfaces', () => {
  assert.match(source, /let installedPluginsOpen = \$state\(false\)/)
  assert.match(source, /let hubPluginsOpen = \$state\(false\)/)
  assert.match(source, /Advanced legacy/)
  assert.match(source, /aria-expanded={installedPluginsOpen}/)
  assert.match(source, /aria-expanded={hubPluginsOpen}/)
})
