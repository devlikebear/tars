import test from 'node:test'
import assert from 'node:assert/strict'

import {
  EMBODIMENT_PROVIDER_CAPABILITY_DETAILS,
  buildLLMProvidersFromDrafts,
  buildLLMTiersFromDrafts,
  buildEmbodimentProvidersFromDrafts,
  extractLLMProviderAliases,
  formatConfigDisplayValue,
  makeEmbodimentProviderDrafts,
  makeEmbodimentProviderPresetDraft,
  makeLLMProviderDrafts,
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
    heavy: {
      provider: 'codex',
      model: 'gpt-5.5',
      reasoning_effort: 'high',
      thinking_budget: 1024,
      service_tier: 'priority',
      max_tokens: 32000,
      beta_features: ['context-1m-2025-08-07', 'interleaved-thinking-2025-05-14'],
    },
    turbo: { provider: 'codex', model: 'gpt-5.4' },
  })

  assert.deepEqual(drafts.map((draft) => draft.name), ['heavy', 'turbo'])
  assert.equal(drafts[0].thinking_budget, '1024')
  assert.equal(drafts[0].service_tier, 'priority')
  assert.equal(drafts[0].max_tokens, '32000')
  assert.equal(drafts[0].beta_features, 'context-1m-2025-08-07\ninterleaved-thinking-2025-05-14')
  assert.equal(drafts[1].reasoning_effort, '')
  assert.equal(drafts[1].max_tokens, '')
  assert.equal(drafts[1].beta_features, '')
})

test('makeLLMTierDrafts renders an unset numeric knob as an empty field', () => {
  // 0 means "unset" on the wire for both knobs; showing a literal 0 would
  // make the operator clear it before they could type a value.
  const drafts = makeLLMTierDrafts({
    heavy: { provider: 'codex', model: 'gpt-5.4', max_tokens: 0 },
  })

  assert.equal(drafts[0].max_tokens, '')
})

test('extractLLMProviderAliases returns sorted provider keys', () => {
  assert.deepEqual(extractLLMProviderAliases({
    minimax: { kind: 'anthropic' },
    codex: { kind: 'openai-codex' },
  }), ['codex', 'minimax'])
})

test('buildLLMTiersFromDrafts validates and serializes tier rows', () => {
  const invalid = buildLLMTiersFromDrafts([
    { id: 'a', originalName: 'heavy', name: 'heavy', provider: 'codex', model: '', reasoning_effort: 'high', thinking_budget: '0', service_tier: '', max_tokens: '', beta_features: '' },
    { id: 'b', originalName: 'dupe', name: 'heavy', provider: 'missing', model: 'gpt-5.4', reasoning_effort: '', thinking_budget: '-1', service_tier: '', max_tokens: 'lots', beta_features: '' },
  ], ['codex'])

  assert.equal(invalid.ok, false)
  if (!invalid.ok) {
    assert.match(invalid.errors.a.model, /required/)
    assert.match(invalid.errors.b.name, /unique/)
    assert.match(invalid.errors.b.provider, /configured provider/)
    assert.match(invalid.errors.b.thinking_budget, /0 or greater/)
    assert.match(invalid.errors.b.max_tokens, /0 or greater/)
  }

  const valid = buildLLMTiersFromDrafts([
    { id: 'heavy', originalName: 'heavy', name: 'heavy', provider: 'codex', model: 'gpt-5.5', reasoning_effort: 'high', thinking_budget: '2048', service_tier: 'priority', max_tokens: '32000', beta_features: 'context-1m-2025-08-07, interleaved-thinking-2025-05-14' },
    { id: 'turbo', originalName: '', name: 'turbo', provider: 'codex', model: 'gpt-5.4', reasoning_effort: '', thinking_budget: '', service_tier: '', max_tokens: '', beta_features: '' },
  ], ['codex'])

  assert.equal(valid.ok, true)
  if (valid.ok) {
    assert.deepEqual(valid.value, {
      heavy: {
        provider: 'codex',
        model: 'gpt-5.5',
        reasoning_effort: 'high',
        thinking_budget: 2048,
        service_tier: 'priority',
        max_tokens: 32000,
        beta_features: ['context-1m-2025-08-07', 'interleaved-thinking-2025-05-14'],
      },
      turbo: {
        provider: 'codex',
        model: 'gpt-5.4',
        reasoning_effort: '',
        thinking_budget: 0,
        service_tier: '',
        max_tokens: 0,
        beta_features: [],
      },
    })
  }
})

test('buildLLMTiersFromDrafts rejects a thinking budget that will not fit under max tokens', () => {
  // Mirrors the resolver guard so the operator sees it before saving
  // rather than as a failure to boot.
  const tooBig = buildLLMTiersFromDrafts([
    { id: 'heavy', originalName: 'heavy', name: 'heavy', provider: 'codex', model: 'gpt-5.5', reasoning_effort: '', thinking_budget: '8000', service_tier: '', max_tokens: '8000', beta_features: '' },
  ], ['codex'])

  assert.equal(tooBig.ok, false)
  if (!tooBig.ok) {
    assert.match(tooBig.errors.heavy.thinking_budget, /less than max tokens/)
  }

  // An unset ceiling resolves to a per-model default the console cannot
  // see, so the guard must not fire.
  const unsetCeiling = buildLLMTiersFromDrafts([
    { id: 'heavy', originalName: 'heavy', name: 'heavy', provider: 'codex', model: 'gpt-5.5', reasoning_effort: '', thinking_budget: '100000', service_tier: '', max_tokens: '', beta_features: '' },
  ], ['codex'])

  assert.equal(unsetCeiling.ok, true)
})

test('buildLLMTiersFromDrafts splits beta features on commas and newlines and de-duplicates', () => {
  const built = buildLLMTiersFromDrafts([
    { id: 'heavy', originalName: 'heavy', name: 'heavy', provider: 'codex', model: 'gpt-5.5', reasoning_effort: '', thinking_budget: '', service_tier: '', max_tokens: '', beta_features: ' a ,\n b\n\na,, ' },
  ], ['codex'])

  assert.equal(built.ok, true)
  if (built.ok) {
    assert.deepEqual(built.value.heavy.beta_features, ['a', 'b'])
  }
})

test('makeLLMProviderDrafts converts provider settings into editable rows', () => {
  const drafts = makeLLMProviderDrafts({
    codex: {
      kind: 'openai-codex',
      auth_mode: 'oauth',
      base_url: 'https://chatgpt.com/backend-api',
      api_key: '',
    },
    anthropic: { kind: 'anthropic', api_key: 'sk-abc' },
  })

  assert.deepEqual(drafts.map((draft) => draft.alias), ['anthropic', 'codex'])
  assert.equal(drafts[0].kind, 'anthropic')
  assert.equal(drafts[0].api_key, 'sk-abc')
  assert.equal(drafts[1].auth_mode, 'oauth')
})

test('buildLLMProvidersFromDrafts validates required fields and uniqueness', () => {
  const invalid = buildLLMProvidersFromDrafts([
    { id: 'a', originalAlias: 'codex', alias: '', kind: 'openai-codex', auth_mode: 'oauth', base_url: '', api_key: '' },
    { id: 'b', originalAlias: 'dup', alias: 'shared', kind: '', auth_mode: '', base_url: '', api_key: '' },
    { id: 'c', originalAlias: 'dup2', alias: 'shared', kind: 'anthropic', auth_mode: '', base_url: '', api_key: '' },
  ])

  assert.equal(invalid.ok, false)
  if (!invalid.ok) {
    assert.match(invalid.errors.a.alias, /required/)
    assert.match(invalid.errors.b.alias, /unique/)
    assert.match(invalid.errors.b.kind, /required/)
    assert.match(invalid.errors.c.alias, /unique/)
  }
})

test('buildLLMProvidersFromDrafts serializes provider drafts into settings map', () => {
  const result = buildLLMProvidersFromDrafts([
    {
      id: 'codex',
      originalAlias: 'codex',
      alias: ' codex ',
      kind: ' openai-codex ',
      auth_mode: 'oauth',
      base_url: 'https://chatgpt.com/backend-api',
      api_key: 'cx-token',
    },
    {
      id: 'anth',
      originalAlias: '',
      alias: 'anthropic',
      kind: 'anthropic',
      auth_mode: 'api-key',
      base_url: '',
      api_key: '',
    },
  ])

  assert.equal(result.ok, true)
  if (result.ok) {
    assert.deepEqual(result.value, {
      codex: {
        kind: 'openai-codex',
        auth_mode: 'oauth',
        base_url: 'https://chatgpt.com/backend-api',
        api_key: 'cx-token',
      },
      anthropic: {
        kind: 'anthropic',
        auth_mode: 'api-key',
        base_url: '',
        api_key: '',
      },
    })
  }
})

test('makeEmbodimentProviderDrafts converts provider descriptors into editable rows', () => {
  const drafts = makeEmbodimentProviderDrafts([
    {
      name: 'host',
      enabled: true,
      transport: 'webhook',
      endpoint: 'http://127.0.0.1:43180/v1/embodiment/percept/host',
      capabilities: ['hearing', 'speech'],
      session_id: 'sess_main',
      owner_only_directive: true,
      salience_min_sound_level: 0.6,
      min_trigger_interval: '30s',
      max_triggers_per_hour: 60,
      trigger_observations: false,
    },
  ])

  assert.equal(drafts.length, 1)
  assert.equal(drafts[0].name, 'host')
  assert.equal(drafts[0].enabled, true)
  assert.deepEqual(drafts[0].capabilities, ['hearing', 'speech'])
  assert.equal(drafts[0].salience_min_sound_level, '0.6')
  assert.equal(drafts[0].max_triggers_per_hour, '60')
})

test('makeEmbodimentProviderPresetDraft recommends Mac Host and StackChan defaults', () => {
  const host = makeEmbodimentProviderPresetDraft('host', 'preset-host')
  assert.equal(host.name, 'host')
  assert.equal(host.transport, 'mcp')
  assert.equal(host.endpoint, 'tars-stackchan-host')
  assert.deepEqual(host.capabilities, ['hearing', 'speech'])
  assert.equal(host.session_id, 'sess_main')
  assert.equal(host.owner_only_directive, false)
  assert.equal(host.trigger_observations, true)

  const stackchan = makeEmbodimentProviderPresetDraft('stackchan', 'preset-stackchan')
  assert.equal(stackchan.name, 'stackchan')
  assert.equal(stackchan.transport, 'mcp')
  assert.equal(stackchan.endpoint, 'tars-stackchan')
  assert.deepEqual(stackchan.capabilities, ['vision', 'hearing', 'speech', 'expression', 'motion', 'led'])
  assert.equal(stackchan.owner_only_directive, true)
  assert.equal(stackchan.trigger_observations, false)

  const duplicate = makeEmbodimentProviderPresetDraft('stackchan', 'preset-stackchan-2', ['stackchan'])
  assert.equal(duplicate.name, 'stackchan2')
  assert.equal(duplicate.endpoint, 'tars-stackchan')
})

test('embodiment capability metadata describes each known capability', () => {
  const detailsByID = new Map(EMBODIMENT_PROVIDER_CAPABILITY_DETAILS.map((detail) => [detail.id, detail]))
  for (const capability of ['vision', 'hearing', 'speech', 'expression', 'motion', 'led']) {
    const detail = detailsByID.get(capability)
    assert.ok(detail, `${capability} should have metadata`)
    assert.ok(detail.label.length > 0)
    assert.ok(detail.description.length > 0)
  }
  assert.equal(detailsByID.get('vision')?.group, 'perception')
  assert.equal(detailsByID.get('speech')?.group, 'actuation')
})

test('buildEmbodimentProvidersFromDrafts validates and serializes provider rows', () => {
  const invalid = buildEmbodimentProvidersFromDrafts([
    {
      id: 'a',
      originalName: 'host',
      name: '',
      enabled: true,
      transport: 'mcp',
      endpoint: '',
      capabilities: [],
      session_id: '',
      agent: '',
      owner_only_directive: false,
      salience_min_sound_level: 'loud',
      min_trigger_interval: '',
      max_triggers_per_hour: '-1',
      trigger_observations: false,
    },
    {
      id: 'b',
      originalName: '',
      name: 'host',
      enabled: true,
      transport: 'mcp',
      endpoint: 'stackchan',
      capabilities: ['speech'],
      session_id: '',
      agent: '',
      owner_only_directive: false,
      salience_min_sound_level: '',
      min_trigger_interval: '',
      max_triggers_per_hour: '',
      trigger_observations: false,
    },
    {
      id: 'c',
      originalName: '',
      name: 'host',
      enabled: true,
      transport: 'mcp',
      endpoint: 'stackchan',
      capabilities: ['speech'],
      session_id: '',
      agent: '',
      owner_only_directive: false,
      salience_min_sound_level: '',
      min_trigger_interval: '',
      max_triggers_per_hour: '',
      trigger_observations: false,
    },
  ])

  assert.equal(invalid.ok, false)
  if (!invalid.ok) {
    assert.match(invalid.errors.a.name, /required/)
    assert.match(invalid.errors.a.endpoint, /required/)
    assert.match(invalid.errors.a.capabilities, /capability/)
    assert.match(invalid.errors.a.salience_min_sound_level, /number/)
    assert.match(invalid.errors.a.max_triggers_per_hour, /0 or greater/)
    assert.match(invalid.errors.b.name, /unique/)
    assert.match(invalid.errors.c.name, /unique/)
  }

  const valid = buildEmbodimentProvidersFromDrafts([
    {
      id: 'host',
      originalName: 'host',
      name: ' host ',
      enabled: true,
      transport: 'webhook',
      endpoint: ' http://127.0.0.1:43180/v1/embodiment/percept/host ',
      capabilities: ['hearing', 'speech', 'hearing'],
      session_id: ' sess_main ',
      agent: 'worker',
      owner_only_directive: true,
      salience_min_sound_level: '0.6',
      min_trigger_interval: '30s',
      max_triggers_per_hour: '60',
      trigger_observations: true,
    },
  ])

  assert.equal(valid.ok, true)
  if (valid.ok) {
    assert.deepEqual(valid.value, [{
      name: 'host',
      enabled: true,
      transport: 'webhook',
      endpoint: 'http://127.0.0.1:43180/v1/embodiment/percept/host',
      capabilities: ['hearing', 'speech'],
      session_id: 'sess_main',
      agent: 'worker',
      owner_only_directive: true,
      salience_min_sound_level: 0.6,
      min_trigger_interval: '30s',
      max_triggers_per_hour: 60,
      trigger_observations: true,
    }])
  }
})
