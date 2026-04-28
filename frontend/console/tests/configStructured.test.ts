import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildLLMTiersFromDrafts,
  extractLLMProviderAliases,
  formatConfigDisplayValue,
  makeLLMTierDrafts,
  parseStructuredJSONEdit,
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

test('parseStructuredJSONEdit validates JSON before applying edits', () => {
  const ok = parseStructuredJSONEdit('{\n  "light": {"provider": "codex"}\n}')
  assert.deepEqual(ok, { ok: true, value: { light: { provider: 'codex' } } })

  const invalid = parseStructuredJSONEdit('{"light": ')
  assert.equal(invalid.ok, false)
  if (!invalid.ok) {
    assert.match(invalid.error, /JSON/)
  }
})

test('makeLLMTierDrafts converts tier bindings into editable rows', () => {
  const drafts = makeLLMTierDrafts({
    heavy: { provider: 'codex', model: 'gpt-5.5', reasoning_effort: 'high', thinking_budget: 1024, service_tier: 'priority' },
    turbo: { provider: 'codex', model: 'gpt-5.4' },
  })

  assert.deepEqual(drafts.map((draft) => draft.name), ['heavy', 'turbo'])
  assert.equal(drafts[0].thinking_budget, '1024')
  assert.equal(drafts[0].service_tier, 'priority')
  assert.equal(drafts[1].reasoning_effort, '')
})

test('extractLLMProviderAliases returns sorted provider keys', () => {
  assert.deepEqual(extractLLMProviderAliases({
    minimax: { kind: 'anthropic' },
    codex: { kind: 'openai-codex' },
  }), ['codex', 'minimax'])
})

test('buildLLMTiersFromDrafts validates and serializes tier rows', () => {
  const invalid = buildLLMTiersFromDrafts([
    { id: 'a', originalName: 'heavy', name: 'heavy', provider: 'codex', model: '', reasoning_effort: 'high', thinking_budget: '0', service_tier: '' },
    { id: 'b', originalName: 'dupe', name: 'heavy', provider: 'missing', model: 'gpt-5.4', reasoning_effort: '', thinking_budget: '-1', service_tier: '' },
  ], ['codex'])

  assert.equal(invalid.ok, false)
  if (!invalid.ok) {
    assert.match(invalid.errors.a.model, /required/)
    assert.match(invalid.errors.b.name, /unique/)
    assert.match(invalid.errors.b.provider, /configured provider/)
    assert.match(invalid.errors.b.thinking_budget, /0 or greater/)
  }

  const valid = buildLLMTiersFromDrafts([
    { id: 'heavy', originalName: 'heavy', name: 'heavy', provider: 'codex', model: 'gpt-5.5', reasoning_effort: 'high', thinking_budget: '2048', service_tier: 'priority' },
    { id: 'turbo', originalName: '', name: 'turbo', provider: 'codex', model: 'gpt-5.4', reasoning_effort: '', thinking_budget: '', service_tier: '' },
  ], ['codex'])

  assert.equal(valid.ok, true)
  if (valid.ok) {
    assert.deepEqual(valid.value, {
      heavy: { provider: 'codex', model: 'gpt-5.5', reasoning_effort: 'high', thinking_budget: 2048, service_tier: 'priority' },
      turbo: { provider: 'codex', model: 'gpt-5.4', reasoning_effort: '', thinking_budget: 0, service_tier: '' },
    })
  }
})
