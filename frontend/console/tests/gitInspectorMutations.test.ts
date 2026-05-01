import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const inspectorSource = readFileSync(new URL('../src/components/GitInspector.svelte', import.meta.url), 'utf8')
const opsSource = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Git Inspector queues mutating actions as approvals', () => {
  assert.match(typesSource, /GitMutationPlan/)
  assert.match(apiSource, /createGitMutationApproval/)
  assert.match(apiSource, /\/v1\/git\/mutations/)
  assert.match(inspectorSource, /requestMutation\('stage'/)
  assert.match(inspectorSource, /requestMutation\('unstage'/)
  assert.match(inspectorSource, /requestMutation\('discard'/)
  assert.match(inspectorSource, /requestMutation\('switch_branch'/)
  assert.match(inspectorSource, /Commit staged/)
  assert.match(opsSource, /git_mutation/)
  assert.match(opsSource, /destructive git action/)
})
