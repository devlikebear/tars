import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildConfigPayload,
  defaultBaseURLForKind,
  emptyOnboardingForm,
  suggestedAuthModeForKind,
  validateForm,
  validateProviderStep,
  validateTiersStep,
} from '../src/lib/onboarding.ts'

test('emptyOnboardingForm returns blank state with all 3 tier slots', () => {
  const form = emptyOnboardingForm()
  assert.equal(form.provider.alias, '')
  assert.equal(form.provider.kind, '')
  assert.equal(form.provider.auth_mode, 'api-key')
  assert.deepEqual(Object.keys(form.tiers).sort(), ['heavy', 'light', 'standard'])
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

test('suggestedAuthModeForKind defaults oauth for codex/claude-code-cli, api-key elsewhere', () => {
  assert.equal(suggestedAuthModeForKind('openai-codex'), 'oauth')
  assert.equal(suggestedAuthModeForKind('claude-code-cli'), 'oauth')
  assert.equal(suggestedAuthModeForKind('openai'), 'api-key')
  assert.equal(suggestedAuthModeForKind('anthropic'), 'api-key')
})
