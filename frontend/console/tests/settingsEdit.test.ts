import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const source = readFileSync(new URL('../src/components/Config.svelte',import.meta.url),'utf8')
test('settings exposes schema search, sections and inline JSON editing', () => {
 assert.match(source,/schema\.filter/)
 assert.match(source,/searchSettings/)
 assert.match(source,/sectionSettings/)
 assert.match(source,/JSON\.parse\(editValue\)/)
 assert.match(source,/startEdit\(field\)/)
 assert.match(source,/provider\.api_key\.includes\('\*'\)/)
 assert.match(source,/field\.sensitive && editValue === ''/)
})
test('save/apply confirms restart and checks new runtime rather than saved values', () => {
 assert.match(source,/if \(saveAndApply\) restartConfirm = true/)
 assert.match(source,/confirmRestart/)
 assert.match(source,/status\.runtime_started_at !== previousRuntime/)
 assert.match(source,/AbortSignal\.timeout/)
 assert.match(source,/reconnectFailed/)
})
