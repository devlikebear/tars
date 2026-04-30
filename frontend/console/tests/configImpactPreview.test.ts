import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildConfigImpactPreview, parseDurationSeconds } from '../src/lib/configImpact.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

const configSource = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('config impact preview estimates pulse interval effects', () => {
  const field: ConfigFieldMeta = {
    key: 'pulse_interval',
    path: 'pulse.interval',
    section: 'Automation',
    type: 'string',
    label: 'Pulse Interval',
    description: 'Pulse tick interval',
    impact: ['Changing this changes the pulse tick cadence.'],
  }

  const preview = buildConfigImpactPreview(field, '60s', '30s')

  assert.equal(parseDurationSeconds('1m'), 60)
  assert.ok(preview.items.some((item) => item.includes('Changing this changes the pulse tick cadence.')))
  assert.ok(preview.items.some((item) => item.includes('Signal detection latency can improve by about 30s.')))
  assert.ok(preview.items.some((item) => item.includes('Pulse tick volume changes by about 2x.')))
})

test('Settings diff panel renders impact preview metadata', () => {
  assert.match(typesSource, /impact\?: string\[\]/)
  assert.match(configSource, /buildConfigImpactPreview/)
  assert.match(configSource, /diff-impact/)
  assert.match(configSource, /Impact/)
})
