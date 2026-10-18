import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildLLMEditorValues,
  buildLLMProvidersValue,
  buildLLMRoleDefaultsValue,
  buildLLMTiersValue,
  createLLMProviderRow,
  createLLMRoleRow,
  llmEditorStateFromValues,
  llmProviderKinds,
  llmProviderRowsFromValue,
  llmRoleRowsFromValue,
  llmTierNames,
  llmTierRowsFromValue,
  renameLLMProviderAlias,
} from '../src/lib/llmConfigEditor.ts'

test('llmProviderKinds includes antigravity-cli', () => {
  assert.ok(llmProviderKinds.includes('antigravity-cli'))
})

// --- values -> rows -------------------------------------------------------

test('llmProviderRowsFromValue sorts aliases and maps fields', () => {
  const rows = llmProviderRowsFromValue({
    zeta: { kind: 'openai', auth_mode: 'api-key', api_key: 'sk-z', base_url: 'https://z.example' },
    alpha: { kind: 'anthropic', auth_mode: 'oauth', api_key: '', base_url: '' },
  })
  assert.deepEqual(rows.map((r) => r.alias), ['alpha', 'zeta'])
  assert.equal(rows[0].kind, 'anthropic')
  assert.equal(rows[0].auth_mode, 'oauth')
  assert.equal(rows[1].api_key, 'sk-z')
  assert.equal(rows[1].base_url, 'https://z.example')
  assert.equal(rows[1].previousAlias, 'zeta')
})

test('llmProviderRowsFromValue tolerates missing/invalid input', () => {
  assert.deepEqual(llmProviderRowsFromValue(null), [])
  assert.deepEqual(llmProviderRowsFromValue(undefined), [])
  const rows = llmProviderRowsFromValue({ p: null })
  assert.equal(rows.length, 1)
  assert.equal(rows[0].kind, '')
  assert.equal(rows[0].auth_mode, 'api-key')
})

test('createLLMProviderRow starts blank with a stable id shape', () => {
  const row = createLLMProviderRow()
  assert.equal(row.alias, '')
  assert.equal(row.kind, '')
  assert.equal(row.auth_mode, 'api-key')
  assert.ok(row.id.startsWith('provider-'))
})

test('llmTierRowsFromValue always returns exactly heavy/standard/light in order', () => {
  const rows = llmTierRowsFromValue({ standard: { provider: 'p', model: 'm' } })
  assert.deepEqual(rows.map((r) => r.name), [...llmTierNames])
  const standard = rows.find((r) => r.name === 'standard')
  assert.equal(standard?.provider, 'p')
  assert.equal(standard?.model, 'm')
  const heavy = rows.find((r) => r.name === 'heavy')
  assert.equal(heavy?.provider, '')
})

test('llmTierRowsFromValue carries unexposed fields through as extra', () => {
  const rows = llmTierRowsFromValue({
    heavy: { provider: 'p', model: 'm', thinking_budget: 4096, service_tier: 'priority', max_tokens: 8192 },
  })
  const heavy = rows.find((r) => r.name === 'heavy')
  assert.equal(heavy?.max_tokens, '8192')
  assert.deepEqual(heavy?.extra, { thinking_budget: 4096, service_tier: 'priority' })
})

test('llmRoleRowsFromValue sorts roles and maps tiers', () => {
  const rows = llmRoleRowsFromValue({ pulse_decider: 'light', chat_main: 'standard' })
  assert.deepEqual(rows.map((r) => r.role), ['chat_main', 'pulse_decider'])
  assert.equal(rows[0].tier, 'standard')
  assert.equal(rows[1].previousRole, 'pulse_decider')
})

test('createLLMRoleRow starts blank', () => {
  const row = createLLMRoleRow()
  assert.equal(row.role, '')
  assert.equal(row.tier, '')
})

// --- rows -> values + validation ------------------------------------------

test('buildLLMProvidersValue rejects empty alias, bad chars, duplicates', () => {
  const dup = createLLMProviderRow()
  dup.alias = 'dup'
  dup.kind = 'anthropic'
  dup.auth_mode = 'oauth'
  const dup2 = { ...dup, id: 'provider-x' }
  const blank = createLLMProviderRow()
  const badChars = createLLMProviderRow()
  badChars.alias = 'has space'
  badChars.kind = 'openai'
  badChars.api_key = 'sk-x'

  const { value, errors } = buildLLMProvidersValue([blank, badChars, dup, dup2])
  assert.ok(errors.some((e) => /alias is required/i.test(e)))
  assert.ok(errors.some((e) => /alphanumeric/i.test(e)))
  assert.ok(errors.some((e) => /used more than once/i.test(e)))
  assert.deepEqual(Object.keys(value), ['dup'])
})

test('buildLLMProvidersValue rejects unknown kind and missing api_key for api-key auth', () => {
  const unknownKind = createLLMProviderRow()
  unknownKind.alias = 'weird'
  unknownKind.kind = 'totally-unknown' as never
  const noKey = createLLMProviderRow()
  noKey.alias = 'nokey'
  noKey.kind = 'openai'
  noKey.auth_mode = 'api-key'
  noKey.api_key = ''

  const { errors } = buildLLMProvidersValue([unknownKind, noKey])
  assert.ok(errors.some((e) => /unknown provider kind/i.test(e)))
  assert.ok(errors.some((e) => /API key is required/i.test(e)))
})

test('buildLLMProvidersValue allows oauth/cli auth without an api_key, and omits the never-set key', () => {
  const row = createLLMProviderRow()
  row.alias = 'codex'
  row.kind = 'openai-codex'
  row.auth_mode = 'oauth'
  const { value, errors } = buildLLMProvidersValue([row])
  assert.deepEqual(errors, [])
  // No api_key key at all — matches internal/config/schema.go's
  // sparseProvidersMap, which omits a never-set field rather than writing
  // an explicit empty string.
  assert.deepEqual(value.codex, { kind: 'openai-codex', auth_mode: 'oauth' })
})

test('buildLLMProvidersValue clears a previously-set api_key with an explicit empty string', () => {
  const rows = llmProviderRowsFromValue({ p: { kind: 'openai', auth_mode: 'oauth', api_key: 'sk-old' } })
  rows[0].api_key = ''
  const { value, errors } = buildLLMProvidersValue(rows)
  assert.deepEqual(errors, [])
  // Present and empty, not omitted: internal/config/patch_raw_providers.go's
  // mergeRawProviderEdit treats an *absent* api_key as "keep the old
  // secret" — omitting it here would silently undo the clear.
  assert.equal((value.p as Record<string, unknown>).api_key, '')
})

test('buildLLMProvidersValue submits the real api_key verbatim on an untouched row (no masking)', () => {
  const rows = llmProviderRowsFromValue({
    p: { kind: 'openai', auth_mode: 'api-key', api_key: 'sk-real-secret', base_url: 'https://api.openai.com/v1' },
  })
  const { value, errors } = buildLLMProvidersValue(rows)
  assert.deepEqual(errors, [])
  assert.equal((value.p as Record<string, unknown>).api_key, 'sk-real-secret')
})

test('buildLLMProvidersValue rebuilds an untouched row to the exact shape it loaded from', () => {
  // internal/config/schema.go's sparseProvidersMap only includes a field
  // when it is non-empty — the rebuilt value for an unedited row must
  // match that shape key-for-key, same reasoning as the tiers round-trip
  // test below (tars#1286).
  const original = { kind: 'openai', auth_mode: 'api-key', api_key: 'sk-real-secret', base_url: 'https://api.openai.com/v1' }
  const rows = llmProviderRowsFromValue({ p: original })
  const { value, errors } = buildLLMProvidersValue(rows)
  assert.deepEqual(errors, [])
  assert.deepEqual(value.p, original)
})

test('buildLLMProvidersValue omits base_url and a never-set api_key for a fresh cli-mode row', () => {
  const row = createLLMProviderRow()
  row.alias = 'bare'
  row.kind = 'claude-code-cli'
  row.auth_mode = 'cli'
  const { value } = buildLLMProvidersValue([row])
  assert.deepEqual(value.bare, { kind: 'claude-code-cli', auth_mode: 'cli' })
})

test('buildLLMTiersValue flags missing/unknown provider and missing model', () => {
  const rows = llmTierRowsFromValue({})
  const { errors } = buildLLMTiersValue(rows, new Set(['known']))
  assert.ok(errors.some((e) => /heavy: provider is required/i.test(e)))
  assert.ok(errors.some((e) => /heavy: model is required/i.test(e)))

  const withUnknown = llmTierRowsFromValue({ heavy: { provider: 'ghost', model: 'm' } })
  const { errors: errors2 } = buildLLMTiersValue(withUnknown, new Set(['known']))
  assert.ok(errors2.some((e) => /heavy: provider "ghost" is not one of the providers/i.test(e)))
})

test('buildLLMTiersValue validates max_tokens as a positive integer and preserves extra fields', () => {
  const rows = llmTierRowsFromValue({
    heavy: { provider: 'p', model: 'm', thinking_budget: 2048, max_tokens: 'not-a-number' },
  })
  const { value, errors } = buildLLMTiersValue(rows, new Set(['p']))
  assert.ok(errors.some((e) => /heavy: max tokens must be a positive integer/i.test(e)))
  const heavy = value.heavy as Record<string, unknown>
  assert.equal(heavy.thinking_budget, 2048)
  assert.equal(heavy.provider, 'p')
  assert.equal(heavy.model, 'm')
  // Invalid input does not get written; it zeroes out rather than vanishing
  // (see buildLLMTiersValue's comment on why the key is never deleted).
  assert.equal(heavy.max_tokens, 0)
})

test('buildLLMTiersValue zeroes reasoning_effort/max_tokens when cleared, never deletes the keys', () => {
  const rows = llmTierRowsFromValue({ standard: { provider: 'p', model: 'm', reasoning_effort: 'high', max_tokens: 4096 } })
  const cleared = rows.map((r) => (r.name === 'standard' ? { ...r, reasoning_effort: '', max_tokens: '' } : r))
  const { value } = buildLLMTiersValue(cleared, new Set(['p']))
  const standard = value.standard as Record<string, unknown>
  assert.equal(standard.reasoning_effort, '')
  assert.equal(standard.max_tokens, 0)
})

test('buildLLMTiersValue rebuilds an untouched row to the exact shape it loaded from', () => {
  // internal/config's LLMTierBinding has no `omitempty` json tags, so
  // GET /schema always returns every field (zero values included) — the
  // rebuilt value for an unedited row must match that shape key-for-key,
  // or ConfigLLMEditor's "did this section actually change" check
  // (lib/configStructured.ts's configValuesEqual) would see a spurious
  // diff on every unrelated keystroke and emit it anyway (tars#1286).
  const original = {
    provider: 'p',
    model: 'm',
    reasoning_effort: '',
    thinking_budget: 0,
    service_tier: '',
    max_tokens: 0,
    beta_features: null,
    context_window: 0,
  }
  const rows = llmTierRowsFromValue({ heavy: original, standard: original, light: original })
  const { value, errors } = buildLLMTiersValue(rows, new Set(['p']))
  assert.deepEqual(errors, [])
  assert.deepEqual(value.heavy, original)
})

test('buildLLMRoleDefaultsValue flags missing role/tier, duplicates, unrecognized role, unknown tier', () => {
  const dup1 = createLLMRoleRow()
  dup1.role = 'chat_main'
  dup1.tier = 'standard'
  const dup2 = createLLMRoleRow()
  dup2.role = 'chat_main'
  dup2.tier = 'light'
  const blankTier = createLLMRoleRow()
  blankTier.role = 'pulse_decider'
  const unknownRole = createLLMRoleRow()
  unknownRole.role = 'made_up_role'
  unknownRole.tier = 'light'
  const badTier = createLLMRoleRow()
  badTier.role = 'critic'
  badTier.tier = 'ultra'

  const { value, errors } = buildLLMRoleDefaultsValue([dup1, dup2, blankTier, unknownRole, badTier])
  assert.ok(errors.some((e) => /set more than once/i.test(e)))
  assert.ok(errors.some((e) => /pulse_decider.*pick a tier/i.test(e)))
  assert.ok(errors.some((e) => /unrecognized role/i.test(e)))
  assert.ok(errors.some((e) => /unknown tier "ultra"/i.test(e)))
  assert.equal(value.chat_main, 'standard')
  assert.equal(value.made_up_role, 'light')
  assert.equal('critic' in value, false)
})

// --- cross-field builder ---------------------------------------------------

test('buildLLMEditorValues validates tier provider references against edited (not original) provider aliases', () => {
  const state = llmEditorStateFromValues(
    { old: { kind: 'openai', auth_mode: 'api-key', api_key: 'sk-x' } },
    { heavy: { provider: 'old', model: 'm' }, standard: { provider: 'old', model: 'm' }, light: { provider: 'old', model: 'm' } },
    {},
  )
  // Rename "old" -> "new" in the provider row and propagate to tiers, as
  // the editor component would on an alias edit.
  state.providers[0].alias = 'new'
  state.tiers = renameLLMProviderAlias(state.tiers, 'old', 'new')

  const result = buildLLMEditorValues(state)
  assert.deepEqual(result.providers.errors, [])
  assert.deepEqual(result.tiers.errors, [])
  assert.equal((result.tiers.value.heavy as Record<string, unknown>).provider, 'new')
})

test('buildLLMEditorValues reports a tier pointing at a deleted provider', () => {
  const state = llmEditorStateFromValues(
    { solo: { kind: 'openai', auth_mode: 'api-key', api_key: 'sk-x' } },
    { heavy: { provider: 'solo', model: 'm' }, standard: { provider: 'solo', model: 'm' }, light: { provider: 'solo', model: 'm' } },
    {},
  )
  state.providers = [] // user deleted the only provider row
  const result = buildLLMEditorValues(state)
  assert.ok(result.tiers.errors.some((e) => /not one of the providers/i.test(e)))
})

test('renameLLMProviderAlias only rewrites tiers pointing at the previous alias, and is a no-op without a change', () => {
  const tiers = llmTierRowsFromValue({
    heavy: { provider: 'a', model: 'm' },
    standard: { provider: 'b', model: 'm' },
    light: { provider: 'a', model: 'm' },
  })
  const renamed = renameLLMProviderAlias(tiers, 'a', 'c')
  assert.equal(renamed.find((r) => r.name === 'heavy')?.provider, 'c')
  assert.equal(renamed.find((r) => r.name === 'light')?.provider, 'c')
  assert.equal(renamed.find((r) => r.name === 'standard')?.provider, 'b')

  const same = renameLLMProviderAlias(tiers, 'a', 'a')
  assert.strictEqual(same, tiers)
  const noPrev = renameLLMProviderAlias(tiers, '', 'c')
  assert.strictEqual(noPrev, tiers)
})
