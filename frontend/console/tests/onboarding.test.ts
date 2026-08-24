import test from 'node:test'
import assert from 'node:assert/strict'

import {
  availableAuthModesForKind,
  buildChannelsPayload,
  buildConfigPayload,
  buildIntegrationsPayload,
  buildSectionPayload,
  buildToolsPayload,
  channelsFromConfigValues,
  defaultBaseURLForKind,
  emptyOnboardingForm,
  formatPrivateHostAllowlistInput,
  formFromConfigValues,
  integrationsFromConfigValues,
  parsePrivateHostAllowlistInput,
  suggestedAuthModeForKind,
  toolsFromConfigValues,
  validateChannelsStep,
  validateForm,
  validateIntegrationsStep,
  validateProviderStep,
  validateSectionStep,
  validateTiersStep,
  validateToolsStep,
} from '../src/lib/onboarding.ts'
import { sortStrings } from '../src/lib/sort.js'

test('emptyOnboardingForm returns blank state with all 3 tier slots', () => {
  const form = emptyOnboardingForm()
  assert.equal(form.provider.alias, '')
  assert.equal(form.provider.kind, '')
  assert.equal(form.provider.auth_mode, 'api-key')
  assert.deepEqual(sortStrings(Object.keys(form.tiers)), ['heavy', 'light', 'standard'])
})

test('validateProviderStep flags missing alias / kind / api_key', () => {
  const form = emptyOnboardingForm()
  const errs = validateProviderStep(form)
  assert.ok(errs.some((e) => /alias is required/i.test(e)))
  assert.ok(errs.some((e) => /provider kind/i.test(e)))
  assert.ok(errs.some((e) => /api key is required/i.test(e)))
})

test('validateProviderStep rejects non-alphanumeric alias', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'has space'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  const errs = validateProviderStep(form)
  assert.ok(errs.some((e) => /alphanumeric/i.test(e)))
})

test('validateProviderStep allows oauth without api_key', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'codex'
  form.provider.kind = 'openai-codex'
  form.provider.auth_mode = 'oauth'
  const errs = validateProviderStep(form)
  assert.equal(errs.length, 0)
})

test('validateTiersStep requires provider+model on every tier', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  const errs = validateTiersStep(form)
  // 3 tiers * 2 fields (provider + model) = 6 errors
  assert.equal(errs.length, 6, errs.join(' | '))
})

test('validateTiersStep rejects tier provider that does not match wizard alias', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'not-the-alias'
    form.tiers[tier].model = 'gpt-x'
  }
  const errs = validateTiersStep(form)
  assert.ok(errs.length >= 3, 'expected one error per mismatching tier')
  assert.ok(errs.every((e) => /not configured/i.test(e)))
})

test('validateTiersStep accepts complete bindings', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'openai'
    form.tiers[tier].model = 'gpt-5.4'
  }
  assert.equal(validateTiersStep(form).length, 0)
})

test('validateForm aggregates provider + tiers errors', () => {
  const form = emptyOnboardingForm()
  const errs = validateForm(form)
  // provider step: 3 errors, tiers step: 6 errors = 9 total
  assert.equal(errs.length, 9, errs.join(' | '))
})

test('buildConfigPayload shapes a clean PATCH updates map', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  form.provider.base_url = 'https://api.openai.com/v1'
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'openai'
    form.tiers[tier].model = tier === 'light' ? 'gpt-5.4-mini' : 'gpt-5.4'
  }
  form.tiers.heavy.reasoning_effort = 'high'

  const payload = buildConfigPayload(form)
  assert.deepEqual(payload, {
    llm_providers: {
      openai: {
        kind: 'openai',
        auth_mode: 'api-key',
        api_key: 'sk-test',
        base_url: 'https://api.openai.com/v1',
      },
    },
    llm_tiers: {
      heavy: { provider: 'openai', model: 'gpt-5.4', reasoning_effort: 'high' },
      standard: { provider: 'openai', model: 'gpt-5.4' },
      light: { provider: 'openai', model: 'gpt-5.4-mini' },
    },
  })
})

// The alias-keyed PATCH replaces the on-disk tier set with whatever the patch
// sends, exactly as it does for providers. Config.svelte now deep links here
// for llm_tiers editing, so dropping unknown tiers would silently delete them.
test('buildConfigPayload preserves custom tiers already on disk', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-test'
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'openai'
    form.tiers[tier].model = 'gpt-5.4'
  }

  const payload = buildConfigPayload(form, {}, {
    heavy: { provider: 'openai', model: 'stale' },
    vision: { provider: 'gemini', model: 'gemini-3-pro' },
  }) as { llm_tiers: Record<string, unknown> }

  assert.deepEqual(payload.llm_tiers.vision, { provider: 'gemini', model: 'gemini-3-pro' })
  assert.deepEqual(payload.llm_tiers.heavy, { provider: 'openai', model: 'gpt-5.4' })
})

test('buildConfigPayload omits empty optional fields', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'codex'
  form.provider.kind = 'openai-codex'
  form.provider.auth_mode = 'oauth'
  // No api_key, no base_url, no reasoning_effort
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'codex'
    form.tiers[tier].model = 'gpt-5.4'
  }
  const payload = buildConfigPayload(form) as {
    llm_providers: { codex: Record<string, unknown> }
    llm_tiers: Record<string, Record<string, unknown>>
  }
  assert.equal(payload.llm_providers.codex.kind, 'openai-codex')
  assert.equal(payload.llm_providers.codex.auth_mode, 'oauth')
  assert.equal('api_key' in payload.llm_providers.codex, false)
  assert.equal('base_url' in payload.llm_providers.codex, false)
  assert.equal('reasoning_effort' in payload.llm_tiers.heavy, false)
})

test('defaultBaseURLForKind returns canonical URLs and empty for local', () => {
  assert.equal(defaultBaseURLForKind('anthropic'), 'https://api.anthropic.com')
  assert.equal(defaultBaseURLForKind('openai'), 'https://api.openai.com/v1')
  assert.equal(defaultBaseURLForKind('claude-code-cli'), '')
  assert.equal(defaultBaseURLForKind(''), '')
})

test('defaultBaseURLForKind matches backend llmdefaults for gemini variants', () => {
  // Mirrors internal/llmdefaults/defaults.go:GeminiBaseURL & GeminiNativeBaseURL.
  // If the backend constants change, update both — wizard-prefilled URLs that
  // miss the path return 404 with an empty body (#671).
  assert.equal(
    defaultBaseURLForKind('gemini'),
    'https://generativelanguage.googleapis.com/v1beta/openai',
  )
  assert.equal(
    defaultBaseURLForKind('gemini-native'),
    'https://generativelanguage.googleapis.com/v1beta',
  )
})

test('availableAuthModesForKind narrows the auth_mode select per kind', () => {
  assert.deepEqual(availableAuthModesForKind('openai-codex'), ['oauth'])
  assert.deepEqual(availableAuthModesForKind('claude-code-cli'), ['cli'])
  assert.deepEqual(availableAuthModesForKind('anthropic'), ['api-key', 'oauth'])
  assert.deepEqual(availableAuthModesForKind('openai'), ['api-key'])
  assert.deepEqual(availableAuthModesForKind('kimi'), ['api-key'])
  assert.deepEqual(availableAuthModesForKind('gemini'), ['api-key'])
  assert.deepEqual(availableAuthModesForKind('gemini-native'), ['api-key'])
  // Empty kind: every mode is offered until the user picks one.
  assert.deepEqual(availableAuthModesForKind(''), ['api-key', 'oauth', 'cli'])
})

test('suggestedAuthModeForKind matches the head of availableAuthModesForKind', () => {
  assert.equal(suggestedAuthModeForKind('openai-codex'), 'oauth')
  assert.equal(suggestedAuthModeForKind('claude-code-cli'), 'cli', 'cli kinds must default to cli, not oauth')
  assert.equal(suggestedAuthModeForKind('openai'), 'api-key')
  assert.equal(suggestedAuthModeForKind('anthropic'), 'api-key')
})

test('buildConfigPayload never emits oauth_provider — backend infers from kind', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'codex'
  form.provider.kind = 'openai-codex'
  form.provider.auth_mode = 'oauth'
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'codex'
    form.tiers[tier].model = 'gpt-5.4'
  }
  const payload = buildConfigPayload(form) as {
    llm_providers: { codex: Record<string, unknown> }
  }
  assert.equal('oauth_provider' in payload.llm_providers.codex, false)
})

test('validateProviderStep accepts cli auth_mode without api_key', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'claude'
  form.provider.kind = 'claude-code-cli'
  form.provider.auth_mode = 'cli'
  form.provider.api_key = ''
  assert.equal(validateProviderStep(form).length, 0)
})

test('formFromConfigValues prefills the first provider and 3 tiers', () => {
  const values = {
    llm_providers: {
      openai: {
        kind: 'openai',
        auth_mode: 'api-key',
        api_key: 'sk-t****ast4',
        base_url: 'https://api.openai.com/v1',
      },
    },
    llm_tiers: {
      heavy: { provider: 'openai', model: 'gpt-5.4', reasoning_effort: 'high' },
      standard: { provider: 'openai', model: 'gpt-5.4' },
      light: { provider: 'openai', model: 'gpt-5.4-mini' },
    },
  }
  const form = formFromConfigValues(values)
  assert.equal(form.provider.alias, 'openai')
  assert.equal(form.provider.kind, 'openai')
  assert.equal(form.provider.auth_mode, 'api-key')
  assert.equal(form.provider.base_url, 'https://api.openai.com/v1')
  assert.equal(form.provider.keepExistingApiKey, true, 'masked api_key should set keepExisting=true')
  assert.equal(form.tiers.heavy.model, 'gpt-5.4')
  assert.equal(form.tiers.heavy.reasoning_effort, 'high')
  assert.equal(form.tiers.standard.provider, 'openai')
  assert.equal(form.tiers.light.model, 'gpt-5.4-mini')
})

test('formFromConfigValues handles empty values gracefully', () => {
  const form = formFromConfigValues({})
  assert.equal(form.provider.alias, '')
  assert.equal(form.tiers.heavy.provider, '')
})

test('buildConfigPayload omits api_key when keepExistingApiKey is set', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = '****abcd'
  form.provider.keepExistingApiKey = true
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'openai'
    form.tiers[tier].model = 'gpt-5.4'
  }
  const payload = buildConfigPayload(form) as {
    llm_providers: { openai: Record<string, unknown> }
  }
  assert.equal('api_key' in payload.llm_providers.openai, false, 'masked key must not be patched back')
})

test('buildConfigPayload includes api_key when user supplies a fresh value', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.api_key = 'sk-fresh-replacement'
  form.provider.keepExistingApiKey = false
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier].provider = 'openai'
    form.tiers[tier].model = 'gpt-5.4'
  }
  const payload = buildConfigPayload(form) as {
    llm_providers: { openai: Record<string, unknown> }
  }
  assert.equal(payload.llm_providers.openai.api_key, 'sk-fresh-replacement')
})

test('validateProviderStep accepts api-key mode without typed key when keepExisting=true', () => {
  const form = emptyOnboardingForm()
  form.provider.alias = 'openai'
  form.provider.kind = 'openai'
  form.provider.auth_mode = 'api-key'
  form.provider.api_key = '' // user did not type anything
  form.provider.keepExistingApiKey = true
  assert.equal(validateProviderStep(form).length, 0)
})

// --- Tools section ---

test('emptyOnboardingForm initializes new sections with safe defaults', () => {
  const form = emptyOnboardingForm()
  assert.equal(form.tools.web_search_enabled, false)
  assert.deepEqual(form.tools.web_fetch_private_host_allowlist, [])
  assert.equal(form.integrations.memory_embed_dimensions, null)
  assert.equal(form.channels.telegram_enabled, false)
})

test('validateToolsStep blocks save when web_search enabled without api_key', () => {
  const form = emptyOnboardingForm()
  form.tools.web_search_enabled = true
  const errs = validateToolsStep(form)
  assert.ok(errs.some((e) => /api key/i.test(e)), errs.join(' | '))
})

test('validateToolsStep accepts web_search enabled with keepExisting flag', () => {
  const form = emptyOnboardingForm()
  form.tools.web_search_enabled = true
  form.tools.web_search_api_key = '****abcd'
  form.tools.keepWebSearchKey = true
  assert.equal(validateToolsStep(form).length, 0)
})

test('validateToolsStep allows disabled web_search with no key', () => {
  const form = emptyOnboardingForm()
  form.tools.web_fetch_enabled = true
  assert.equal(validateToolsStep(form).length, 0)
})

test('buildToolsPayload always emits boolean keys + JSON-encodes the allowlist', () => {
  const form = emptyOnboardingForm()
  form.tools.web_search_enabled = true
  form.tools.web_search_provider = 'tavily'
  form.tools.web_search_api_key = 'tvly-fresh'
  form.tools.web_fetch_enabled = true
  form.tools.web_fetch_allow_private_hosts = true
  form.tools.web_fetch_private_host_allowlist = ['10.0.0.1', 'internal.local']
  form.tools.allow_high_risk_user = true
  const payload = buildToolsPayload(form)
  assert.equal(payload.tools_web_search_enabled, true)
  assert.equal(payload.tools_web_fetch_enabled, true)
  assert.equal(payload.tools_web_fetch_allow_private_hosts, true)
  assert.equal(payload.tools_allow_high_risk_user, true)
  assert.equal(payload.tools_web_search_provider, 'tavily')
  assert.equal(payload.tools_web_search_api_key, 'tvly-fresh')
  assert.equal(
    payload.tools_web_fetch_private_host_allowlist_json,
    JSON.stringify(['10.0.0.1', 'internal.local']),
  )
})

test('buildToolsPayload omits api_key when keepWebSearchKey is set', () => {
  const form = emptyOnboardingForm()
  form.tools.web_search_enabled = true
  form.tools.web_search_api_key = '****abcd'
  form.tools.keepWebSearchKey = true
  const payload = buildToolsPayload(form)
  assert.equal('tools_web_search_api_key' in payload, false, 'masked key must not be patched back')
})

// --- Integrations section ---

test('validateIntegrationsStep treats memory_embed as opt-in', () => {
  const form = emptyOnboardingForm()
  // No provider set → no error even when api_key empty
  assert.equal(validateIntegrationsStep(form).length, 0)
})

test('validateIntegrationsStep flags missing api_key when provider is set', () => {
  const form = emptyOnboardingForm()
  form.integrations.memory_embed_provider = 'gemini'
  const errs = validateIntegrationsStep(form)
  assert.ok(errs.some((e) => /api key/i.test(e)), errs.join(' | '))
})

test('buildIntegrationsPayload omits empty fields and api_key when keep flag set', () => {
  const form = emptyOnboardingForm()
  form.integrations.memory_embed_provider = 'gemini'
  form.integrations.memory_embed_api_key = '****key1'
  form.integrations.keepMemoryEmbedKey = true
  form.integrations.memory_embed_model = 'gemini-embedding-001'
  form.integrations.memory_embed_dimensions = 768
  const payload = buildIntegrationsPayload(form)
  assert.equal(payload.memory_embed_provider, 'gemini')
  assert.equal(payload.memory_embed_model, 'gemini-embedding-001')
  assert.equal(payload.memory_embed_dimensions, 768)
  assert.equal('memory_embed_api_key' in payload, false)
  assert.equal('memory_embed_base_url' in payload, false)
})

// --- Channels section ---

test('validateChannelsStep blocks telegram enable without bot token', () => {
  const form = emptyOnboardingForm()
  form.channels.telegram_enabled = true
  const errs = validateChannelsStep(form)
  assert.ok(errs.some((e) => /bot token/i.test(e)), errs.join(' | '))
})

test('validateChannelsStep blocks polling without telegram enabled', () => {
  const form = emptyOnboardingForm()
  form.channels.telegram_polling_enabled = true
  const errs = validateChannelsStep(form)
  assert.ok(errs.some((e) => /polling requires the telegram channel/i.test(e)))
})

test('validateChannelsStep accepts complete telegram config', () => {
  const form = emptyOnboardingForm()
  form.channels.telegram_enabled = true
  form.channels.telegram_bot_token = '12345:ABCDEF'
  form.channels.telegram_polling_enabled = true
  assert.equal(validateChannelsStep(form).length, 0)
})

test('buildChannelsPayload omits bot token when keepTelegramToken is set', () => {
  const form = emptyOnboardingForm()
  form.channels.telegram_enabled = true
  form.channels.telegram_bot_token = '****CDEF'
  form.channels.keepTelegramToken = true
  const payload = buildChannelsPayload(form)
  assert.equal(payload.channels_telegram_enabled, true)
  assert.equal('telegram_bot_token' in payload, false, 'masked token must not be patched back')
})

test('buildChannelsPayload always includes the boolean toggles', () => {
  const form = emptyOnboardingForm()
  form.channels.webhook_enabled = true
  const payload = buildChannelsPayload(form)
  assert.equal(payload.channels_telegram_enabled, false)
  assert.equal(payload.channels_telegram_polling_enabled, false)
  assert.equal(payload.channels_webhook_enabled, true)
})

// --- Section dispatch + reentry ---

test('buildSectionPayload dispatches per section', () => {
  const form = emptyOnboardingForm()
  form.tools.web_fetch_enabled = true
  const tools = buildSectionPayload('tools', form) as Record<string, unknown>
  assert.equal(tools.tools_web_fetch_enabled, true)
  // Tools payload must NOT include channels keys.
  assert.equal('channels_telegram_enabled' in tools, false)
})

test('validateSectionStep dispatches per section', () => {
  const form = emptyOnboardingForm()
  form.channels.telegram_enabled = true
  const errs = validateSectionStep('channels', form)
  assert.ok(errs.length > 0)
})

test('toolsFromConfigValues marks keepWebSearchKey when masked api_key present', () => {
  const tools = toolsFromConfigValues({
    tools_web_search_enabled: true,
    tools_web_search_api_key: 'tvly****1234',
    tools_web_fetch_private_host_allowlist_json: '["10.0.0.1","internal.local"]',
  })
  assert.equal(tools.web_search_enabled, true)
  assert.equal(tools.keepWebSearchKey, true)
  assert.deepEqual(tools.web_fetch_private_host_allowlist, ['10.0.0.1', 'internal.local'])
})

test('integrationsFromConfigValues parses dimensions as number', () => {
  const i = integrationsFromConfigValues({
    memory_embed_provider: 'gemini',
    memory_embed_dimensions: 768,
  })
  assert.equal(i.memory_embed_provider, 'gemini')
  assert.equal(i.memory_embed_dimensions, 768)
})

test('channelsFromConfigValues sets keepTelegramToken when token present', () => {
  const c = channelsFromConfigValues({
    channels_telegram_enabled: true,
    telegram_bot_token: '****ABCD',
  })
  assert.equal(c.telegram_enabled, true)
  assert.equal(c.keepTelegramToken, true)
})

test('channelsFromConfigValues clears stale polling when channel is disabled on disk', () => {
  // Inconsistent on-disk state: polling=true but channel=false. The
  // wizard hides the polling checkbox in this state (no UI affordance
  // to flip it), so loading it as-is would trap the user behind the
  // "polling requires channel enabled" validator. Normalize on load.
  const c = channelsFromConfigValues({
    channels_telegram_enabled: false,
    channels_telegram_polling_enabled: true,
  })
  assert.equal(c.telegram_enabled, false)
  assert.equal(c.telegram_polling_enabled, false, 'polling must be cleared when channel is off')
})

test('channelsFromConfigValues preserves polling when channel is enabled on disk', () => {
  const c = channelsFromConfigValues({
    channels_telegram_enabled: true,
    channels_telegram_polling_enabled: true,
    telegram_bot_token: '****ABCD',
  })
  assert.equal(c.telegram_enabled, true)
  assert.equal(c.telegram_polling_enabled, true)
})

test('parsePrivateHostAllowlistInput strips comments and blanks', () => {
  const input = '\n10.0.0.1\n# comment line\n\ninternal.local  \n'
  assert.deepEqual(parsePrivateHostAllowlistInput(input), ['10.0.0.1', 'internal.local'])
})

test('formatPrivateHostAllowlistInput round-trips a host list', () => {
  const formatted = formatPrivateHostAllowlistInput(['10.0.0.1', 'internal.local'])
  assert.equal(formatted, '10.0.0.1\ninternal.local')
})

test('formFromConfigValues prefills tools / integrations / channels', () => {
  const form = formFromConfigValues({
    tools_web_search_enabled: true,
    tools_web_search_api_key: 'tvly****',
    memory_embed_provider: 'gemini',
    memory_embed_api_key: 'sk-emb****',
    channels_telegram_enabled: true,
    telegram_bot_token: '****CDEF',
  })
  assert.equal(form.tools.web_search_enabled, true)
  assert.equal(form.tools.keepWebSearchKey, true)
  assert.equal(form.integrations.memory_embed_provider, 'gemini')
  assert.equal(form.integrations.keepMemoryEmbedKey, true)
  assert.equal(form.channels.telegram_enabled, true)
  assert.equal(form.channels.keepTelegramToken, true)
})
