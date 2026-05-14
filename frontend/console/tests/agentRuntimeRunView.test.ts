import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/AgentRuntimeRunView.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const costFlowSource = readFileSync(new URL('../src/components/AgentRuntimeCostFlow.svelte', import.meta.url), 'utf8')
const replaySource = readFileSync(new URL('../src/components/AgentRuntimeReplay.svelte', import.meta.url), 'utf8')
const flowGraphSource = readFileSync(new URL('../src/components/AgentRuntimeFlowGraph.svelte', import.meta.url), 'utf8')
const treeSource = readFileSync(new URL('../src/components/AgentRuntimeTree.svelte', import.meta.url), 'utf8')
const ganttSource = readFileSync(new URL('../src/components/AgentRuntimeGantt.svelte', import.meta.url), 'utf8')
const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('Agent Runtime runs page exposes filters, session links, and cost summaries', () => {
  assert.match(source, /runStatusFilter/)
  assert.match(source, /runTimeRange/)
  assert.match(source, /runSearchInput/)
  assert.match(source, /Status/)
  assert.match(source, /24h/)
  assert.match(source, /7d/)
  assert.match(i18nEnSource, /filterSearchLabel: 'Search prompt'/)
  assert.match(source, /filterSearchLabel/)
  assert.match(i18nEnSource, /sessionLink: \(id\) => `Started from session/)
  assert.match(source, /sessionLink\(shortID/)
  assert.match(source, /\/console\/chat\/\$\{encodeURIComponent/)
  assert.match(source, /run\.session_id/)
  assert.match(source, /cost-summary-card/)
  assert.match(source, /Today/)
  assert.match(i18nEnSource, /costPlanTotals: 'Plan totals'/)
  assert.match(source, /costPlanTotals/)
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

test('Agent Runtime runs page exposes static tree and Gantt visualization modes', () => {
  assert.match(source, /AgentRuntimeTree/)
  assert.match(source, /AgentRuntimeGantt/)
  assert.match(source, /runViewMode/)
  assert.match(source, /Tree/)
  assert.match(source, /Gantt/)
  assert.match(treeSource, /agent-runtime-tree/)
  assert.match(treeSource, /Mini Tree/)
  assert.match(ganttSource, /agent-runtime-gantt/)
  assert.match(ganttSource, /Gantt Strip/)
})

test('Agent Runtime runs page exposes Svelte Flow live graph mode', () => {
  assert.match(source, /AgentRuntimeFlowGraph/)
  assert.match(source, /flow/)
  assert.match(flowGraphSource, /SvelteFlow/)
  assert.match(flowGraphSource, /MiniMap/)
  assert.match(flowGraphSource, /Controls/)
  assert.match(flowGraphSource, /Background/)
  assert.match(flowGraphSource, /flowTierFilter/)
  assert.match(flowGraphSource, /flowStatusFilter/)
  assert.match(flowGraphSource, /flowSessionFilter/)
  assert.match(flowGraphSource, /Replay/)
})
