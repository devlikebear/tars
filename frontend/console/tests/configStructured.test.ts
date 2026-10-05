import test from 'node:test'
import assert from 'node:assert/strict'

import {
  formatConfigDisplayValue,
  prettyConfigJSON,
} from '../src/lib/configStructured.ts'

test('formatConfigDisplayValue summarizes object values without compact JSON blobs', () => {
  const summary = formatConfigDisplayValue({
    heavy: { provider: 'codex', model: 'gpt-5.5' },
    light: { provider: 'codex', model: 'gpt-5.4' },
    standard: { provider: 'codex', model: 'gpt-5.5' },
  })

  assert.equal(summary.kind, 'object')
  assert.equal(summary.text, '3 keys')
  assert.deepEqual(summary.preview, ['heavy', 'light', 'standard'])
  assert.equal(summary.raw.includes('{"heavy"'), false)
})

test('formatConfigDisplayValue summarizes arrays with item count and preview', () => {
  const summary = formatConfigDisplayValue(['read_file', 'list_dir', 'glob', 'web_fetch'])

  assert.equal(summary.kind, 'array')
  assert.equal(summary.text, '4 items')
  assert.deepEqual(summary.preview, ['read_file', 'list_dir', 'glob'])
})

test('prettyConfigJSON returns stable indented JSON for structured values', () => {
  const text = prettyConfigJSON({ light: { provider: 'codex', model: 'gpt-5.4' } })

  assert.match(text, /^\{\n  "light": \{/)
  assert.match(text, /"model": "gpt-5.4"/)
})
