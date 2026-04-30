import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/AgentRuntimeRunView.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const costFlowSource = readFileSync(new URL('../src/components/AgentRuntimeCostFlow.svelte', import.meta.url), 'utf8')
const replaySource = readFileSync(new URL('../src/components/AgentRuntimeReplay.svelte', import.meta.url), 'utf8')

test('Agent Runtime runs page exposes filters, session links, and cost summaries', () => {
  assert.match(source, /runStatusFilter/)
  assert.match(source, /runTimeRange/)
  assert.match(source, /runSearchInput/)
  assert.match(source, /Status/)
  assert.match(source, /24h/)
  assert.match(source, /7d/)
  assert.match(source, /Search prompt/)
  assert.match(source, /Started from session/)
  assert.match(source, /\/console\/chat\/\$\{encodeURIComponent/)
  assert.match(source, /run\.session_id/)
  assert.match(source, /cost-summary-card/)
  assert.match(source, /Today/)
  assert.match(source, /Plan totals/)
})

test('Agent Runtime run API client forwards filter query params', () => {
  assert.match(apiSource, /AgentRuntimeRunsOptions/)
  assert.match(apiSource, /status/)
  assert.match(apiSource, /since/)
  assert.match(apiSource, /search/)
  assert.match(apiSource, /URLSearchParams/)
})

test('Agent Runtime run detail exposes file attention heatmap data', () => {
  assert.match(source, /fileAttentionRows/)
  assert.match(source, /File Attention/)
  assert.match(source, /file-heatmap/)
  assert.match(source, /sparkline/)
  assert.match(source, /file_attention/)
  assert.match(source, /tool\.call/)
})

test('Agent Runtime run detail exposes token and cost flow visualization', () => {
  assert.match(source, /AgentRuntimeCostFlow/)
  assert.match(source, /costFlowRuns/)
  assert.match(source, /cost-flow-panel/)
  assert.match(costFlowSource, /Actual cost/)
  assert.match(costFlowSource, /Tokens/)
  assert.match(costFlowSource, /Token and cost Sankey diagram/)
})

test('Agent Runtime run detail exposes replay scrubber controls', () => {
  assert.match(source, /AgentRuntimeReplay/)
  assert.match(source, /replayEvents/)
  assert.match(replaySource, /replay-scrubber/)
  assert.match(replaySource, /Live/)
  assert.match(replaySource, /5x/)
})
