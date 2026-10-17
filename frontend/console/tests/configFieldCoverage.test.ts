// Every basic/advanced field in the server schema needs a Korean
// label+description override in i18n/sections/config.ts (#1241) — this test
// reads the generated docs/config-field-tiers.md (kept current by
// `go test ./internal/config -run TestConfigFieldTierDocIsCurrent`, whose
// Key column was added for this check) rather than talking to a live
// server, so it fails the moment a new basic/advanced field is added
// without a Korean override, no server required.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { configKo } from '../src/i18n/sections/config.ts'

const docPath = new URL('../../../docs/config-field-tiers.md', import.meta.url)

interface DocRow {
  key: string
  tier: string
}

function parseDoc(markdown: string): DocRow[] {
  const rows: DocRow[] = []
  // Section tables look like: | `key` | `yaml.path` | tier | shown when | why |
  const rowPattern = /^\|\s*`([a-z0-9_]+)`\s*\|\s*`[^`]*`\s*\|\s*(basic|advanced|file|remove)\s*\|/gm
  for (const match of markdown.matchAll(rowPattern)) {
    rows.push({ key: match[1], tier: match[2] })
  }
  return rows
}

test('docs/config-field-tiers.md has the Key column this test depends on', () => {
  const markdown = readFileSync(docPath, 'utf8')
  assert.match(markdown, /\| Key \| YAML path \| Tier \| Shown when \| Why \|/)
})

test('every basic/advanced field has a Korean label and description override', () => {
  const markdown = readFileSync(docPath, 'utf8')
  const rows = parseDoc(markdown)
  const consoleRows = rows.filter((r) => r.tier === 'basic' || r.tier === 'advanced')
  assert.ok(consoleRows.length > 90, `expected >90 basic/advanced fields, found ${consoleRows.length}`)

  const missing: string[] = []
  for (const row of consoleRows) {
    const override = configKo.fields[row.key]
    if (!override || !override.label.trim() || !override.description.trim()) {
      missing.push(row.key)
    }
  }
  assert.deepEqual(missing, [], `missing Korean config.ts field overrides for: ${missing.join(', ')}`)
})

test('no stale Korean overrides for fields the console no longer shows (file/remove tier or renamed/removed)', () => {
  const markdown = readFileSync(docPath, 'utf8')
  const rows = parseDoc(markdown)
  const consoleKeys = new Set(rows.filter((r) => r.tier === 'basic' || r.tier === 'advanced').map((r) => r.key))
  const stale = Object.keys(configKo.fields).filter((key) => !consoleKeys.has(key))
  assert.deepEqual(stale, [], `stale config.ts field overrides (not basic/advanced in the schema): ${stale.join(', ')}`)
})
