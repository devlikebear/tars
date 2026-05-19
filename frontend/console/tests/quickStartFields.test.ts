import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  buildQuickStartItems,
  quickStartFieldKeys,
  quickStartProgress,
} from '../src/lib/quickStartFields.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

const configSource = readFileSync(new URL('../src/components/Config.svelte', import.meta.url), 'utf8')
const configStructuredSource = readFileSync(new URL('../src/lib/configStructured.ts', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')

function field(key: string, label = key, type = 'string'): ConfigFieldMeta {
  return {
    key,
    path: key.replaceAll('_', '.'),
    section: 'Test',
    type,
    label,
    description: `${label} description`,
    default_value: '',
    requires_restart: true,
  }
}

test('quick start field list is curated and ordered for onboarding', () => {
  assert.deepEqual(quickStartFieldKeys, [
    'api_auth_mode',
    'llm_providers',
    'llm_tiers',
    'llm_default_tier',
    'workspace_dir',
    'telegram_bot_token',
    'companion_enabled',
    'embodiment_enabled',
    'embodiment_providers_json',
    'pulse_enabled',
    'reflection_enabled',
    'log_level',
    'session_telegram_scope',
  ])
})

test('quick start validation highlights required provider and workspace setup', () => {
  const schema = quickStartFieldKeys.map((key) => field(key, key, key.endsWith('_enabled') ? 'bool' : 'string'))
  const values = {
    api_auth_mode: 'off',
    llm_providers: {},
    llm_tiers: { heavy: {}, standard: {} },
    llm_default_tier: 'standard',
    workspace_dir: '',
    companion_enabled: false,
    embodiment_enabled: false,
    embodiment_providers_json: [],
    pulse_enabled: true,
    reflection_enabled: true,
    log_level: 'info',
    session_telegram_scope: 'main',
  }

  const items = buildQuickStartItems(schema, values, {})
  const providers = items.find((item) => item.key === 'llm_providers')
  const workspace = items.find((item) => item.key === 'workspace_dir')

  assert.equal(providers?.status.kind, 'attention')
  assert.match(providers?.status.message || '', /provider/i)
  assert.equal(workspace?.status.kind, 'attention')
  assert.equal(items.find((item) => item.key === 'embodiment_enabled')?.status.kind, 'ready')
  assert.equal(items.find((item) => item.key === 'embodiment_providers_json')?.status.kind, 'optional')
  assert.equal(quickStartProgress(items).total, 10)
})

test('quick start accepts provider credentials and reports progress', () => {
  const schema = quickStartFieldKeys.map((key) => field(key, key, key.endsWith('_enabled') ? 'bool' : 'string'))
  const values = {
    api_auth_mode: 'off',
    llm_providers: { anthropic: { kind: 'anthropic', api_key: 'sk-ant' } },
    llm_tiers: { heavy: {}, standard: {}, light: {} },
    llm_default_tier: 'standard',
    workspace_dir: './workspace',
    telegram_bot_token: '',
    companion_enabled: true,
    embodiment_enabled: true,
    embodiment_providers_json: [{ name: 'host', transport: 'mcp' }],
    pulse_enabled: true,
    reflection_enabled: true,
    log_level: 'info',
    session_telegram_scope: 'main',
  }

  const items = buildQuickStartItems(schema, values, {})
  assert.equal(items.find((item) => item.key === 'llm_providers')?.status.kind, 'ready')
  assert.equal(items.find((item) => item.key === 'embodiment_providers_json')?.status.kind, 'ready')
  assert.equal(quickStartProgress(items).ready, 10)
})

test('Settings renders Quick Start tab and LLM connection action', () => {
  assert.match(apiSource, /getProviderModels/)
  assert.match(configSource, /'quick'/)
  assert.match(configSource, /Quick Start/)
  assert.match(configSource, /quick-start-grid/)
  assert.match(configSource, /Test connection/)
  assert.match(configSource, /openEmbodimentProviderEditor/)
  assert.match(configSource, /EMBODIMENT_PROVIDER_PRESETS/)
  assert.match(configStructuredSource, /Add StackChan/)
  assert.match(configSource, /capability-chip/)
})
