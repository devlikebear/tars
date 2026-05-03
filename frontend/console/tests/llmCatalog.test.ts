import test from 'node:test'
import assert from 'node:assert/strict'

import {
  SNAPSHOT_DATE,
  SNAPSHOT_SOURCE,
  popularModelsForKind,
  wellKnownModelsByKind,
} from '../src/lib/llm-catalog.ts'
import { providerKinds } from '../src/lib/onboarding.ts'

test('snapshot metadata is non-empty so future PRs can grep for the date', () => {
  assert.ok(SNAPSHOT_DATE.length > 0, 'SNAPSHOT_DATE must be set')
  assert.ok(/\d{4}-\d{2}-\d{2}/.test(SNAPSHOT_DATE), 'SNAPSHOT_DATE should look like YYYY-MM-DD')
  assert.ok(SNAPSHOT_SOURCE.length > 0, 'SNAPSHOT_SOURCE must be set')
})

test('every provider kind has at least one well-known model', () => {
  for (const kind of providerKinds) {
    const models = wellKnownModelsByKind[kind]
    assert.ok(Array.isArray(models), `kind ${kind} must have a catalog entry`)
    assert.ok(models.length >= 1, `kind ${kind} must have at least one model`)
  }
})

test('catalog entries omit the openrouter slash prefix', () => {
  for (const [kind, models] of Object.entries(wellKnownModelsByKind)) {
    for (const id of models) {
      assert.equal(
        id.includes('/'),
        false,
        `kind ${kind} model "${id}" leaked an openrouter prefix; native API ids are slash-free`,
      )
    }
  }
})

test('anthropic ids use dash form for the native API', () => {
  for (const id of wellKnownModelsByKind.anthropic) {
    assert.equal(
      id.includes('.'),
      false,
      `anthropic model "${id}" should use dash form (claude-opus-4-7) not dot form for the native API`,
    )
  }
})

test('claude-code-cli catalog only carries short aliases', () => {
  for (const id of wellKnownModelsByKind['claude-code-cli']) {
    assert.equal(
      id.includes('-'),
      false,
      `claude-code-cli model "${id}" must be a short alias (sonnet/opus/haiku)`,
    )
    assert.equal(
      id.includes('.'),
      false,
      `claude-code-cli model "${id}" must not carry a version dot`,
    )
  }
})

test('popularModelsForKind returns the kind catalog or [] for empty kind', () => {
  assert.deepEqual(popularModelsForKind(''), [])
  for (const kind of providerKinds) {
    assert.deepEqual(popularModelsForKind(kind), wellKnownModelsByKind[kind])
  }
})
