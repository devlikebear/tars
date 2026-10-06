import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const source = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
test('Settings uses inline editors rather than heavyweight modal editors', () => {
 assert.doesNotMatch(source, /json-editor-modal|modal-backdrop/)
 assert.match(source, /field.type === 'json'/)
})
test('Settings restores all-field search and section filtering, retaining Quick Start progress', () => {
 assert.match(source, /schema\.filter/)
 assert.match(source, /searchSettings/)
 assert.match(source, /sectionSettings/)
 assert.match(source, /quick-start-panel/)
 assert.doesNotMatch(source, /openJSONWizard/)
})
