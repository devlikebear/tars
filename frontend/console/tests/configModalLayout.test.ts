import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const source = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
// Per-type field editing (json/select/bool/text) now lives in ConfigControl,
// shared by every field row and the search results (#1242).
const controlSource = readFileSync(new URL('../src/components/ConfigControl.svelte', import.meta.url), 'utf8')

test('Settings uses inline editors rather than heavyweight modal editors', () => {
  assert.doesNotMatch(source, /json-editor-modal|modal-backdrop/)
  assert.doesNotMatch(controlSource, /json-editor-modal|modal-backdrop/)
  assert.match(controlSource, /field.type === 'json'/)
})

test('Settings has cross-tab search and section tabs, not the old single Quick Start grid (#1238/#1239)', () => {
  assert.match(source, /searchConfigFields/)
  assert.match(source, /ConfigTabs/)
  assert.match(source, /\$t\.config\.search\.label/)
  assert.doesNotMatch(source, /quick-start-panel/)
  assert.doesNotMatch(source, /openJSONWizard/)
})
