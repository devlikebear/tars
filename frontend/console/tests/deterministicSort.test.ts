import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { compareStrings, sortStrings } from '../src/lib/sort.js'

const sourceFiles = [
  'src/lib/configStructured.ts',
  'src/lib/onboarding.ts',
  'src/lib/sessionPermissionPreview.ts',
  'src/lib/configMetaBadges.ts',
  'src/components/Onboarding.svelte',
  'src/components/AgentRuntimeFlowGraph.svelte',
  'src/components/SessionConfigPanel.svelte',
  'src/components/Config.svelte',
]

test('string sort helpers provide explicit deterministic compare functions', () => {
  assert.deepEqual(['beta', 'alpha', 'gamma'].sort(compareStrings), ['alpha', 'beta', 'gamma'])
  assert.deepEqual(sortStrings(new Set(['zeta', 'alpha', 'beta'])), ['alpha', 'beta', 'zeta'])
})

test('frontend source avoids ambiguous argument-free string sort calls', () => {
  // fileURLToPath, not .pathname: on Windows the latter yields "/C:/..."
  // and join() turns that into "C:\C:\...".
  const root = fileURLToPath(new URL('../', import.meta.url))
  for (const file of sourceFiles) {
    const source = readFileSync(join(root, file), 'utf8')
    assert.equal(source.includes('.sort()'), false, `${file} should pass an explicit comparator`)
  }
})
