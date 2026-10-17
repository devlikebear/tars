import test from 'node:test'
import assert from 'node:assert/strict'

import {
  CONFIG_TABS,
  basicFields,
  advancedFields,
  defaultConfigTabId,
  groupFieldsByTab,
  isConfigTabId,
  isFieldConsoleVisible,
  isFieldShown,
  tabForSection,
} from '../src/lib/configSections.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

function field(overrides: Partial<ConfigFieldMeta>): ConfigFieldMeta {
  return {
    key: 'x',
    path: 'x',
    section: 'Runtime',
    type: 'string',
    label: 'X',
    description: 'x',
    tier: 'basic',
    ...overrides,
  }
}

test('every schema section maps to exactly one tab', () => {
  const sections = [
    'Runtime', 'API', 'Remote Access', 'LLM', 'Memory', 'Usage', 'Automation', 'Compaction',
    'Tools', 'MCP', 'Agent Runtime', 'Work Ledger', 'Work Scheduler', 'Channels', 'Console',
    'Companion', 'Initiative', 'Extensions', 'Embodiment',
  ]
  for (const section of sections) {
    const tabId = tabForSection(section)
    assert.ok(CONFIG_TABS.some((tab) => tab.id === tabId), `${section} -> unknown tab ${tabId}`)
  }
})

test('the advanced tab carries no sections of its own', () => {
  const advancedTab = CONFIG_TABS.find((tab) => tab.id === 'advanced')
  assert.ok(advancedTab)
  assert.deepEqual(advancedTab.sections, [])
})

test('isConfigTabId / defaultConfigTabId round-trip every tab id', () => {
  for (const tab of CONFIG_TABS) assert.ok(isConfigTabId(tab.id))
  assert.equal(isConfigTabId('bogus'), false)
  assert.equal(isConfigTabId(undefined), false)
  assert.ok(isConfigTabId(defaultConfigTabId()))
})

test('file and remove tiers never show in the console', () => {
  assert.equal(isFieldConsoleVisible(field({ tier: 'file' })), false)
  assert.equal(isFieldConsoleVisible(field({ tier: 'remove' })), false)
  assert.equal(isFieldConsoleVisible(field({ tier: 'basic' })), true)
  assert.equal(isFieldConsoleVisible(field({ tier: 'advanced' })), true)
})

test('show_when hides a field until its condition holds', () => {
  const parent = field({ key: 'channels_telegram_enabled', type: 'bool', tier: 'basic' })
  const child = field({
    key: 'session_telegram_scope',
    tier: 'advanced',
    show_when: [{ key: 'channels_telegram_enabled', values: ['true'] }],
  })
  const all = [parent, child]
  assert.equal(isFieldShown(child, { channels_telegram_enabled: false }, all), false)
  assert.equal(isFieldShown(child, { channels_telegram_enabled: true }, all), true)
})

test('show_when with several conditions is an OR, and the named field must itself be shown', () => {
  const a = field({ key: 'a', type: 'select', options: ['x', 'y'], tier: 'basic' })
  const hiddenParent = field({ key: 'hidden_parent', tier: 'file' })
  const child = field({
    key: 'c',
    tier: 'advanced',
    show_when: [{ key: 'a', values: ['y'] }, { key: 'hidden_parent', values: ['true'] }],
  })
  const all = [a, hiddenParent, child]
  assert.equal(isFieldShown(child, { a: 'x', hidden_parent: true }, all), false)
  assert.equal(isFieldShown(child, { a: 'y', hidden_parent: false }, all), true)
  // hidden_parent itself is tier "file", so its condition can never resolve true.
  assert.equal(isFieldShown(child, { a: 'x', hidden_parent: true }, all), false)
})

test('groupFieldsByTab splits by tab, hides file/remove, and respects show_when', () => {
  const fields = [
    field({ key: 'workspace_dir', section: 'Runtime', tier: 'advanced' }),
    field({ key: 'llm_providers', section: 'LLM', tier: 'basic' }),
    field({ key: 'secret_internal', section: 'Runtime', tier: 'file' }),
    field({
      key: 'dependent',
      section: 'Channels',
      tier: 'advanced',
      show_when: [{ key: 'channel_switch', values: ['true'] }],
    }),
    field({ key: 'channel_switch', section: 'Channels', type: 'bool', tier: 'basic' }),
  ]
  const grouped = groupFieldsByTab(fields, { channel_switch: false })
  const general = grouped.find((g) => g.id === 'general')
  const llm = grouped.find((g) => g.id === 'llm')
  const channelsRemote = grouped.find((g) => g.id === 'channelsRemote')
  assert.ok(general?.fields.some((f) => f.key === 'workspace_dir'))
  assert.ok(!general?.fields.some((f) => f.key === 'secret_internal'))
  assert.ok(llm?.fields.some((f) => f.key === 'llm_providers'))
  assert.ok(channelsRemote?.fields.some((f) => f.key === 'channel_switch'))
  assert.ok(!channelsRemote?.fields.some((f) => f.key === 'dependent'))

  const groupedWithSwitchOn = groupFieldsByTab(fields, { channel_switch: true })
  const channelsRemoteOn = groupedWithSwitchOn.find((g) => g.id === 'channelsRemote')
  assert.ok(channelsRemoteOn?.fields.some((f) => f.key === 'dependent'))
})

test('basicFields / advancedFields split by tier only', () => {
  const fields = [field({ key: 'a', tier: 'basic' }), field({ key: 'b', tier: 'advanced' }), field({ key: 'c', tier: 'file' })]
  assert.deepEqual(basicFields(fields).map((f) => f.key), ['a'])
  assert.deepEqual(advancedFields(fields).map((f) => f.key), ['b'])
})
