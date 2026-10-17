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

// Review findings (tars PR #1283, round 1): a bind:value number input
// silently broke int/float commits, the restart confirmation had no
// cancel, search and the field grid could surface a show_when-hidden or
// RemoteAccessCard-owned field with no row to land on, and blurring
// llm_providers without editing it stripped the masked api_key anyway.
const saveBarSource = readFileSync(new URL('../src/components/ConfigSaveBar.svelte', import.meta.url), 'utf8')

test('the int/float/text input commits via oninput, not bind:value, so Svelte cannot coerce draft away from a string', () => {
  // The textarea (json/string_list) legitimately keeps bind:value — it is
  // never type="number", so Svelte's input-coercion special case never
  // applies to it. Only the single <input> (int/float/text/sensitive)
  // control is at risk, and that one must read e.currentTarget.value.
  assert.match(controlSource, /oninput=\{\(e\) => \{ draft = \(e\.currentTarget as HTMLInputElement\)\.value \}\}/)
  const inputBlock = controlSource.slice(controlSource.indexOf('<input\n'), controlSource.indexOf('/>', controlSource.indexOf('<input\n')))
  assert.doesNotMatch(inputBlock, /bind:value/)
})

test('an untouched control commits nothing on blur', () => {
  assert.match(controlSource, /if \(draft === syncedText\) return/)
})

test('the restart confirmation can be cancelled', () => {
  assert.match(saveBarSource, /onCancelRestart/)
  assert.match(saveBarSource, /\$t\.config\.restart\.cancel/)
})

test('search and the field grid exclude show_when-hidden and RemoteAccessCard-owned fields', () => {
  assert.match(source, /isFieldShown/)
  assert.match(source, /REMOTE_ACCESS_FIELD_KEYS/)
  assert.match(source, /function isGenericRow/)
  assert.doesNotMatch(source, /fields: schema,/)
})
