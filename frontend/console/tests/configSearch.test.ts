import test from 'node:test'
import assert from 'node:assert/strict'

import { highlightConfigText, searchConfigFields } from '../src/lib/configSearch.ts'
import type { ConfigFieldLocalization } from '../src/lib/configSearch.ts'
import type { ConfigFieldMeta } from '../src/lib/types.ts'

function field(overrides: Partial<ConfigFieldMeta>): ConfigFieldMeta {
  return {
    key: 'x',
    path: 'x.path',
    section: 'Runtime',
    type: 'string',
    label: 'X label',
    description: 'x description',
    tier: 'basic',
    ...overrides,
  }
}

const identityLocalize = (field: ConfigFieldMeta): ConfigFieldLocalization => ({
  label: field.label,
  description: field.description,
  sectionLabel: field.section,
})

test('highlightConfigText marks every case-insensitive occurrence', () => {
  const segments = highlightConfigText('Auto-archive Sessions', 'session')
  assert.deepEqual(segments.map((s) => [s.text, s.matched]), [
    ['Auto-archive ', false],
    ['Session', true],
    ['s', false],
  ])
})

test('highlightConfigText with an empty query marks nothing', () => {
  assert.deepEqual(highlightConfigText('hello', ''), [{ text: 'hello', matched: false }])
})

test('searchConfigFields matches on label, description, path, key, section, and env var name', () => {
  const fields = [
    field({ key: 'session_auto_archive_days', path: 'runtime.session.auto_archive_days', label: 'Auto-archive Sessions', section: 'Runtime' }),
    field({ key: 'llm_default_tier', path: 'llm.default_tier', label: 'Default Tier', description: 'fallback tier', section: 'LLM' }),
  ]
  const base = { fields, values: {}, dirtyFields: {}, envOverrides: {}, localize: identityLocalize }

  assert.equal(searchConfigFields({ ...base, query: 'archive' }).totalCount, 1)
  assert.equal(searchConfigFields({ ...base, query: 'fallback' }).totalCount, 1)
  assert.equal(searchConfigFields({ ...base, query: 'runtime.session.auto_archive_days' }).totalCount, 1)
  assert.equal(searchConfigFields({ ...base, query: 'llm_default_tier' }).totalCount, 1)
  assert.equal(searchConfigFields({ ...base, query: 'LLM' }).totalCount, 1)

  const envResult = searchConfigFields({
    ...base,
    query: 'TARS_LLM_DEFAULT_TIER',
    envOverrides: { llm_default_tier: { env_key: 'TARS_LLM_DEFAULT_TIER' } },
  })
  assert.equal(envResult.totalCount, 1)
  assert.equal(envResult.groups[0].hits[0].field.key, 'llm_default_tier')
})

test('an empty query with no filters returns every console-visible field, grouped by section', () => {
  const fields = [
    field({ key: 'a', section: 'Runtime' }),
    field({ key: 'b', section: 'LLM' }),
    field({ key: 'c', section: 'Runtime' }),
    field({ key: 'd', tier: 'file', section: 'Runtime' }),
  ]
  const result = searchConfigFields({ fields, query: '', values: {}, dirtyFields: {}, envOverrides: {}, localize: identityLocalize })
  assert.equal(result.totalCount, 3)
  const runtimeGroup = result.groups.find((g) => g.section === 'Runtime')
  assert.equal(runtimeGroup?.hits.length, 2)
})

test('quick filters: changed from default, env overridden, unsaved', () => {
  const fields = [
    field({ key: 'changed', default_value: 'a' }),
    field({ key: 'unchanged', default_value: 'a' }),
    field({ key: 'env_only', default_value: 'a' }),
    field({ key: 'dirty_only', default_value: 'a' }),
  ]
  const values = { changed: 'b', unchanged: 'a', env_only: 'a', dirty_only: 'a' }
  const dirtyFields = { dirty_only: 'z' }
  const envOverrides = { env_only: { env_key: 'ENV_ONLY' } }

  const changedResult = searchConfigFields({
    fields, query: '', values, dirtyFields, envOverrides, localize: identityLocalize, filters: ['changedFromDefault'],
  })
  assert.deepEqual(changedResult.groups.flatMap((g) => g.hits.map((h) => h.field.key)), ['changed'])

  const envResult = searchConfigFields({
    fields, query: '', values, dirtyFields, envOverrides, localize: identityLocalize, filters: ['envOverridden'],
  })
  assert.deepEqual(envResult.groups.flatMap((g) => g.hits.map((h) => h.field.key)), ['env_only'])

  const unsavedResult = searchConfigFields({
    fields, query: '', values, dirtyFields, envOverrides, localize: identityLocalize, filters: ['unsaved'],
  })
  assert.deepEqual(unsavedResult.groups.flatMap((g) => g.hits.map((h) => h.field.key)), ['dirty_only'])
})

test('searchConfigFields never returns file/remove-tier fields even on exact key match', () => {
  const fields = [field({ key: 'hidden', tier: 'file', label: 'hidden label' })]
  const result = searchConfigFields({ fields, query: 'hidden', values: {}, dirtyFields: {}, envOverrides: {}, localize: identityLocalize })
  assert.equal(result.totalCount, 0)
})
