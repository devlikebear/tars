import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viewSource = readFileSync(new URL('../src/components/AgentRuntimeRunView.svelte', import.meta.url), 'utf8')
const typeSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Agent Runtime run detail exposes git diff timeline data', () => {
  assert.match(typeSource, /AgentRuntimeDiffTimelineEntry/)
  assert.match(typeSource, /diff_timeline\?: AgentRuntimeDiffTimelineEntry\[\]/)
  assert.match(typeSource, /AgentRuntimeDiffFileChange/)
  assert.match(viewSource, /diffTimelineEntries/)
  assert.match(viewSource, /Diff Timeline/)
  assert.match(viewSource, /diff-timeline/)
  assert.match(viewSource, /Diff preview/)
  assert.match(viewSource, /git_inspector_url/)
  assert.match(viewSource, /\/console\/agentruntime\/runs\/\$\{encodeURIComponent/)
})
