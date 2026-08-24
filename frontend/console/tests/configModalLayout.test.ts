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

test('Config inspection surfaces read-only values with YAML-key pointers and wizard deep links', () => {
  assert.match(source, /config-yaml-view/)
  assert.match(source, /Read-only/)
  assert.match(source, /jsonWizardLink/)
  assert.match(source, /onboarding\?reentry=1&section=provider/)
  assert.match(source, /onboarding\?reentry=1&section=tiers/)
  assert.match(source, /yaml-key-hint/)
  assert.match(source, /config\/tars\.config\.example\.yaml/)
})
