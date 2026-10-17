import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

// Field-type editing moved from Config.svelte's old editingKey/editValue
// state machine into the shared ConfigControl (#1242); Config.svelte keeps
// only page-level concerns (search, tabs, save/restart, the llm_providers
// masked-key guard).
const source = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
const controlSource = readFileSync(new URL('../src/components/ConfigControl.svelte', import.meta.url), 'utf8')

test('settings exposes cross-tab schema search, section tabs, and inline JSON editing', () => {
  assert.match(source, /searchConfigFields/)
  assert.match(source, /ConfigTabs/)
  assert.match(source, /\$t\.config\.search\.label/)
  assert.match(controlSource, /JSON\.parse\(draft\)/)
})

test('a masked llm_providers api_key is stripped before it reaches dirtyFields, never round-tripped as the new secret', () => {
  assert.match(source, /function stripMaskedProviderKeys/)
  assert.match(source, /provider\.api_key\.includes\('\*'\)/)
  assert.match(source, /function setFieldValue\(field: ConfigFieldMeta, rawParsed: unknown\)/)
})

test('save/apply confirms restart and checks new runtime rather than saved values', () => {
  assert.match(source, /if \(saveAndApply\) restartConfirm = true/)
  assert.match(source, /restartConfirm = true/)
  assert.match(source, /\$t\.config\.restart\.confirm/)
  assert.match(source, /status\.runtime_started_at !== previousRuntime/)
  assert.match(source, /AbortSignal\.timeout/)
  assert.match(source, /\$t\.config\.reconnectFailed/)
})
