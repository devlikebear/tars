import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildConfigImpactPreview, parseDurationSeconds } from '../src/lib/configImpact.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

const configSource = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
const pendingChangesSource = readFileSync(new URL('../src/components/ConfigPendingChanges.svelte', import.meta.url), 'utf8')
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

test('config impact preview derives subsystem fallback hints', () => {
  const fields: ConfigFieldMeta[] = [
    {
      key: 'llm_tiers',
      path: 'llm.tiers',
      section: 'LLM',
      type: 'json',
      label: 'Tiers',
      description: 'LLM tier bindings',
    },
    {
      key: 'api_admin_token',
      path: 'api.admin_token',
      section: 'API',
      type: 'string',
      label: 'Admin Token',
      description: 'Admin token',
    },
    {
      key: 'reflection_sleep_window',
      path: 'reflection.sleep_window',
      section: 'Automation',
      type: 'string',
      label: 'Reflection Sleep Window',
      description: 'Reflection window',
    },
    {
      key: 'skills_enabled',
      path: 'skills.enabled',
      section: 'Extensions',
      type: 'bool',
      label: 'Skills Enabled',
      description: 'Load skills',
    },
  ]

  const impacts = fields.flatMap((field) => buildConfigImpactPreview(field, '', 'changed').items)

  assert.ok(impacts.some((item) => item.includes('LLM routing')))
  assert.ok(impacts.some((item) => item.includes('Auth/API')))
  assert.ok(impacts.some((item) => item.includes('Reflection')))
  assert.ok(impacts.some((item) => item.includes('Extensions')))
})

test('Settings diff panel renders impact preview metadata', () => {
  assert.match(typesSource, /impact\?: string\[\]/)
  assert.match(configSource, /buildConfigImpactPreview/)
  assert.match(pendingChangesSource, /diff-impact/)
  assert.match(pendingChangesSource, /Impact/)
})
