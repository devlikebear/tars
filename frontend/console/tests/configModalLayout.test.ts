import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')

test('Config page stays inspection-first and hosts no heavyweight modal editors (#931)', () => {
  // The long-tail config editors (generic JSON, LLM tier/provider, embodiment
  // presets) were removed; provider/tier editing lives in the onboarding
  // wizard reentry and everything else is documented file-first.
  assert.doesNotMatch(source, /json-editor-modal/)
  assert.doesNotMatch(source, /tier-editor|provider-editor|embodiment/i)
  assert.doesNotMatch(source, /modal-backdrop/)
})

test('Config page is Quick Start only after the #931 freeze', () => {
  // The field inspector (165 fields) and the YAML read view are gone; Quick
  // Start checks are the whole page. Everything else is documented file-first.
  assert.doesNotMatch(source, /viewMode|view-toggle|toggle-btn/)
  assert.doesNotMatch(source, /config-yaml-view|inspect-note/)
  assert.match(source, /quick-start-panel/)
  assert.match(source, /jsonWizardLink/)
  assert.match(source, /onboarding\?reentry=1&section=provider/)
  assert.match(source, /onboarding\?reentry=1&section=tiers/)
  assert.match(source, /yaml-key-hint/)
  assert.match(source, /config\/tars\.config\.example\.yaml/)
})
