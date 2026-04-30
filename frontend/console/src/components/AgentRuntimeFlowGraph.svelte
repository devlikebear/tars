<script lang="ts">
  import {
    Background,
    Controls,
    MiniMap,
    SvelteFlow,
    type NodeEventWithPointer,
  } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import {
    buildAgentRuntimeFlowGraph,
    type AgentRuntimeFlowFilters,
    type AgentRuntimeFlowNode,
    type AgentRuntimeStatusKind,
  } from '../lib/agentruntime-graph'
  import type { AgentRuntimeRun } from '../lib/types'

  interface Props {
    runs: AgentRuntimeRun[]
    onSelectRun: (runID: string) => void
  }

  type FlowStatusFilter = 'all' | AgentRuntimeStatusKind

  let { runs, onSelectRun }: Props = $props()
  let flowTierFilter = $state('all')
  let flowStatusFilter: FlowStatusFilter = $state('all')
  let flowSessionFilter = $state('all')

  const statusOptions: { value: FlowStatusFilter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'done', label: 'Done' },
    { value: 'error', label: 'Error' },
    { value: 'pending', label: 'Pending' },
  ]

  let tierOptions = $derived(uniqueOptions(runs.map((run) => run.tier || 'default')))
  let sessionOptions = $derived(uniqueOptions(runs.map((run) => run.session_id || '').filter(Boolean)))
  let flowFilters = $derived.by<AgentRuntimeFlowFilters>(() => ({
    tier: flowTierFilter === 'all' ? undefined : flowTierFilter,
    status: flowStatusFilter === 'all' ? undefined : flowStatusFilter,
    session: flowSessionFilter === 'all' ? undefined : flowSessionFilter,
  }))
  let graph = $derived(buildAgentRuntimeFlowGraph(runs, flowFilters))

  const handleNodeClick: NodeEventWithPointer<MouseEvent | TouchEvent, AgentRuntimeFlowNode> = ({ node }) => {
    onSelectRun(node.data.runId)
  }

  function uniqueOptions(values: string[]): string[] {
    return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort()
  }
</script>

<section class="agent-runtime-flow-graph" aria-label="Agent Runtime Svelte Flow live graph">
  <div class="flow-head">
    <div>
      <h3>Live Graph</h3>
      <p>{graph.nodes.length} nodes / {graph.edges.length} edges</p>
    </div>
    <button class="flow-replay-button" type="button" disabled>Replay</button>
  </div>

  <div class="flow-filter-row" aria-label="Agent Runtime graph filters">
    <div class="filter-group">
      <span class="filter-label">Tier</span>
      <div class="filter-chip-row">
        <button type="button" class="filter-chip" class:active={flowTierFilter === 'all'} onclick={() => (flowTierFilter = 'all')}>All</button>
        {#each tierOptions as tier}
          <button type="button" class="filter-chip" class:active={flowTierFilter === tier} onclick={() => (flowTierFilter = tier)}>{tier}</button>
        {/each}
      </div>
    </div>
    <div class="filter-group">
      <span class="filter-label">Status</span>
      <div class="filter-chip-row">
        {#each statusOptions as option}
          <button type="button" class="filter-chip" class:active={flowStatusFilter === option.value} onclick={() => (flowStatusFilter = option.value)}>{option.label}</button>
        {/each}
      </div>
    </div>
    <label class="session-filter">
      <span class="filter-label">Session</span>
      <select bind:value={flowSessionFilter}>
        <option value="all">All</option>
        {#each sessionOptions as sessionID}
          <option value={sessionID}>{sessionID}</option>
        {/each}
      </select>
    </label>
  </div>

  {#if graph.nodes.length === 0}
    <div class="agentruntime-empty">No runs match the current graph filters.</div>
  {:else}
    <div class="flow-canvas">
      <SvelteFlow
        nodes={graph.nodes}
        edges={graph.edges}
        fitView
        minZoom={0.25}
        maxZoom={1.6}
        onnodeclick={handleNodeClick}
      >
        <MiniMap />
        <Controls />
        <Background />
      </SvelteFlow>
    </div>
  {/if}
</section>

<style>
  .agent-runtime-flow-graph { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); overflow: hidden; }
  .flow-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
  .flow-head h3 { margin: 0; }
  .flow-head p { margin: var(--space-1) 0 0; color: var(--text-tertiary); font-size: var(--text-xs); }
  .flow-replay-button { min-height: 30px; border: 1px solid var(--border-subtle); background: var(--surface-inset); color: var(--text-tertiary); border-radius: var(--radius-sm); padding: 0 var(--space-3); font: inherit; font-size: var(--text-xs); }
  .flow-filter-row { display: grid; grid-template-columns: minmax(160px, auto) minmax(220px, 1fr) minmax(180px, 0.3fr); gap: var(--space-3); align-items: end; }
  .filter-group, .session-filter { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; }
  .filter-label { color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .filter-chip-row { display: flex; flex-wrap: wrap; gap: var(--space-1); }
  .filter-chip { min-height: 28px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-secondary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .filter-chip.active { border-color: var(--primary); background: var(--primary-muted); color: var(--primary-text); }
  .session-filter select { width: 100%; min-height: 30px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-primary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); }
  .flow-canvas { height: 520px; min-height: 360px; overflow: hidden; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  :global(.svelte-flow) { background: transparent; color: var(--text-primary); }
  :global(.svelte-flow__node.agent-flow-node) { width: 190px; border: 1px solid var(--border-default); background: var(--surface-elevated); color: var(--text-primary); font-family: var(--font-mono); font-size: 11px; white-space: pre-line; box-shadow: none; transition: border-color 0.2s ease, transform 0.2s ease; }
  :global(.svelte-flow__node.agent-flow-node:hover) { transform: translateY(-1px); border-color: var(--primary); }
  :global(.svelte-flow__node.flow-node-heavy) { width: 164px; min-height: 164px; border-radius: 999px; display: grid; place-items: center; text-align: center; }
  :global(.svelte-flow__node.flow-node-standard) { border-radius: var(--radius-md); }
  :global(.svelte-flow__node.flow-node-light) { width: 150px; min-height: 42px; border-radius: var(--radius-sm); }
  :global(.svelte-flow__node.flow-node-variant) { width: 158px; min-height: 50px; border-width: 2px; }
  :global(.svelte-flow__node.flow-status-running) { border-color: var(--primary); }
  :global(.svelte-flow__node.flow-status-done) { border-color: var(--success); }
  :global(.svelte-flow__node.flow-status-error) { border-color: var(--error); }
  :global(.svelte-flow__edge.flow-edge-spawn path) { stroke-dasharray: 7 6; stroke-width: 2; }
  :global(.svelte-flow__edge.flow-edge-variant path) { stroke-width: 4; }
  :global(.svelte-flow__edge.flow-status-running path) { stroke: var(--primary); }
  :global(.svelte-flow__edge.flow-status-done path) { stroke: var(--success); }
  :global(.svelte-flow__edge.flow-status-error path) { stroke: var(--error); }
  :global(.svelte-flow__minimap), :global(.svelte-flow__controls) { background: var(--surface); border: 1px solid var(--border-subtle); }
  @media (max-width: 768px) {
    .flow-head { align-items: stretch; flex-direction: column; }
    .flow-filter-row { grid-template-columns: 1fr; }
    .flow-canvas { height: 420px; }
  }
</style>
