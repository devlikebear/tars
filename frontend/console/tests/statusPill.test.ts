import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pillSource = readFileSync(new URL('../src/components/StatusPill.svelte', import.meta.url), 'utf8')
const headerSource = readFileSync(new URL('../src/components/Header.svelte', import.meta.url), 'utf8')
const manifestSource = readFileSync(new URL('../public/manifest.webmanifest', import.meta.url), 'utf8')

test('StatusPill aggregates server, pulse, reflection, and active sessions', () => {
  assert.match(pillSource, /getPulseStatus/)
  assert.match(pillSource, /getReflectionStatus/)
  assert.match(pillSource, /listSessions/)
  assert.match(pillSource, /aggregateLevel/)
  assert.match(pillSource, /activeSession/)
  assert.match(pillSource, /onNavigate\?\.\(`\/console\/chat\//)
  assert.match(pillSource, /POLL_INTERVAL_MS/)
})

test('StatusPill replaces the old header indicator in Header.svelte', () => {
  assert.match(headerSource, /import StatusPill from '\.\/StatusPill\.svelte'/)
  assert.match(headerSource, /<StatusPill \{serverHealth\} \{onNavigate\} \/>/)
  assert.doesNotMatch(headerSource, /class="header-indicator"/)
  assert.doesNotMatch(headerSource, /class="header-dot"/)
})

test('manifest.webmanifest declares shortcuts for Chat, Sessions, Ops, Pulse, Reflection', () => {
  const manifest = JSON.parse(manifestSource)
  assert.ok(Array.isArray(manifest.shortcuts), 'shortcuts must be an array')
  const names = manifest.shortcuts.map((s: { name: string }) => s.name)
  for (const expected of ['Chat', 'Sessions', 'Ops', 'Pulse', 'Reflection']) {
    assert.ok(names.includes(expected), `shortcut "${expected}" must be present, got ${names.join(', ')}`)
  }
  const urls = manifest.shortcuts.map((s: { url: string }) => s.url)
  for (const url of urls) {
    assert.match(url, /^\/console\//, `shortcut url ${url} must be rooted under /console/`)
  }
  for (const shortcut of manifest.shortcuts) {
    assert.ok(Array.isArray(shortcut.icons) && shortcut.icons.length > 0, `shortcut ${shortcut.name} must declare at least one icon`)
  }
})
