import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { agentRuntimeRunEn } from '../src/i18n/sections/agentRuntimeRun.ts'

const viewSource = readFileSync(new URL('../src/components/AgentRuntimeRunView.svelte', import.meta.url), 'utf8')
const typeSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Agent Runtime run detail exposes git diff timeline data', () => {
  assert.match(typeSource, /AgentRuntimeDiffTimelineEntry/)
  assert.match(typeSource, /diff_timeline\?: AgentRuntimeDiffTimelineEntry\[\]/)
  assert.match(typeSource, /AgentRuntimeDiffFileChange/)
  assert.match(viewSource, /diffTimelineEntries/)
  assert.match(viewSource, /\$t\.agentRuntimeRun\.diffTimeline\.title/)
  assert.equal(agentRuntimeRunEn.diffTimeline.title, 'Diff Timeline')
  assert.match(viewSource, /diff-timeline/)
  assert.match(viewSource, /\$t\.agentRuntimeRun\.diffTimeline\.preview/)
  assert.equal(agentRuntimeRunEn.diffTimeline.preview, 'Diff preview')
  assert.match(viewSource, /git_inspector_url/)
  assert.match(viewSource, /\/console\/agentruntime\/runs\/\$\{encodeURIComponent/)
})
