import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')

test('Hub registry entries expose quality metadata fields', () => {
  assert.match(typesSource, /export type HubQualityMetadata/)
  assert.match(typesSource, /score: number/)
  assert.match(typesSource, /last_updated\?: string/)
  assert.match(typesSource, /tests_passing\?: boolean/)
  assert.match(typesSource, /required_tools\?: string\[\]/)
  assert.match(typesSource, /permissions\?: string\[\]/)
  assert.match(typesSource, /companion_cli\?: boolean/)
  assert.match(typesSource, /install_count\?: number/)
  assert.match(typesSource, /quality\?: HubQualityMetadata/)
})

test('Extensions renders quality score and install-time trust signals', () => {
  assert.match(extensionsSource, /function qualityScoreLabel\(entry: HubRegistryEntry\)/)
  assert.match(extensionsSource, /function qualitySignals\(entry: HubRegistryEntry\)/)
  assert.match(extensionsSource, /quality-score/)
  assert.match(extensionsSource, /quality-signals/)
  assert.match(extensionsSource, /Tests passing/)
  assert.match(extensionsSource, /Companion CLI/)
  assert.match(extensionsSource, /Required tools/)
  assert.match(extensionsSource, /Permissions/)
})
