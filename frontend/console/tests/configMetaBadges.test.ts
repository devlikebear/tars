import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildConfigMetaBadges, formatRelativeConfigTime } from '../src/lib/configMetaBadges.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

const configSource = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('config meta badges distinguish defaults, restart fields, and secrets', () => {
  const field: ConfigFieldMeta = {
    key: 'pulse_interval',
    path: 'pulse.interval',
    section: 'Automation',
    type: 'string',
    label: 'Pulse Interval',
    description: 'Pulse tick interval',
    default_value: '1m',
    requires_restart: true,
  }

  const badges = buildConfigMetaBadges(field, '1m', false, '2026-05-01T00:00:00Z', new Date('2026-05-01T00:30:00Z'))
  assert.deepEqual(badges.map((badge) => badge.label), ['default', 'requires restart'])

  const modified = buildConfigMetaBadges({ ...field, sensitive: true }, '30s', true, undefined)
  assert.deepEqual(modified.map((badge) => badge.label), ['modified just now', 'requires restart', 'secret'])
})

test('config meta badges render live and relative modified labels', () => {
  const field: ConfigFieldMeta = {
    key: 'log_level',
    path: 'log.level',
    section: 'Runtime',
    type: 'select',
    label: 'Log Level',
    description: 'Logging verbosity',
    default_value: 'info',
    requires_restart: false,
  }

  const badges = buildConfigMetaBadges(field, 'debug', false, '2026-05-01T00:00:00Z', new Date('2026-05-01T02:00:00Z'))
  assert.deepEqual(badges.map((badge) => badge.label), ['modified 2h ago', 'live'])
  assert.equal(formatRelativeConfigTime('2026-05-01T00:00:00Z', new Date('2026-05-01T00:00:20Z')), 'just now')
})

test('Settings field rows render config meta badge metadata', () => {
  assert.match(typesSource, /default_value\?: unknown/)
  assert.match(typesSource, /requires_restart\?: boolean/)
  assert.match(typesSource, /updated_at\?: string/)
  assert.match(configSource, /buildConfigMetaBadges/)
  assert.match(configSource, /field-meta-badges/)
  assert.match(configSource, /badge-restart/)
})
