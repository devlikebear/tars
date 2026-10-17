import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildConfigMetaBadges } from '../src/lib/configMetaBadges.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

const configRowSource = readFileSync(new URL('../src/components/ConfigFieldRow.svelte', import.meta.url), 'utf8')

const text = {
  changed: 'Changed',
  changedTitleDirty: 'Pending change differs from the default value.',
  changedTitleSaved: 'Current value differs from the default value.',
  secret: 'Secret',
  secretTitle: 'Value is masked in the form view.',
}

test('config meta badges only call out changed and secret fields', () => {
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

  // Matches the default and isn't dirty: no badges at all.
  assert.deepEqual(buildConfigMetaBadges(field, '1m', false, text), [])

  // Saved value differs from default, but not pending: "changed" with the
  // saved-state title.
  const saved = buildConfigMetaBadges(field, '30s', false, text)
  assert.deepEqual(saved.map((b) => b.label), ['Changed'])
  assert.equal(saved[0].title, text.changedTitleSaved)

  // Pending edit: "changed" with the dirty-state title, plus "secret" when
  // the field is sensitive.
  const dirty = buildConfigMetaBadges({ ...field, sensitive: true }, '45s', true, text)
  assert.deepEqual(dirty.map((b) => b.label), ['Changed', 'Secret'])
  assert.equal(dirty[0].title, text.changedTitleDirty)
})

test('secret-only fields still get a badge even when unchanged', () => {
  const field: ConfigFieldMeta = {
    key: 'api_admin_token',
    path: 'api.admin_token',
    section: 'API',
    type: 'string',
    label: 'Admin Token',
    description: 'Admin token',
    sensitive: true,
  }
  const badges = buildConfigMetaBadges(field, 'secret-value', false, text)
  assert.deepEqual(badges.map((b) => b.label), ['Secret'])
})

test('field rows render the simplified meta badges', () => {
  assert.match(configRowSource, /buildConfigMetaBadges/)
  assert.match(configRowSource, /badge-changed/)
})
