<script lang="ts">
  import {
    buildAgentRuntimeTreeRows,
    type AgentRuntimeTreeRow,
  } from '../lib/agentruntime-graph'
  import type { AgentRuntimeRun } from '../lib/types'

  interface Props {
    runs: AgentRuntimeRun[]
    onSelectRun: (runID: string) => void
  }

  let { runs, onSelectRun }: Props = $props()
  let rows = $derived(buildAgentRuntimeTreeRows(runs))
  let height = $derived(Math.max(180, rows.length * 78 + 48))
  let rowMap = $derived.by<Map<string, AgentRuntimeTreeRow>>(() => new Map(rows.map((row) => [row.runId, row])))

  function activateRun(event: KeyboardEvent, runID: string) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onSelectRun(runID)
  }

  function nodeClass(row: AgentRuntimeTreeRow): string {
    return `tree-node ${row.statusKind} ${row.tierShape}`
  }

  function parentRow(row: AgentRuntimeTreeRow): AgentRuntimeTreeRow | null {
    return row.parentRunId ? rowMap.get(row.parentRunId) ?? null : null
  }

  function shortID(value: string): string {
    return value.length > 12 ? `${value.slice(0, 12)}...` : value
  }
</script>

<section class="agent-runtime-tree" aria-label="Agent Runtime Mini Tree">
  <div class="visualization-head">
    <div>
      <h3>Mini Tree</h3>
      <p>{rows.length} runs grouped by parent and depth</p>
    </div>
  </div>

  {#if rows.length === 0}
    <div class="agentruntime-empty">No runs available for tree visualization.</div>
  {:else}
    <div class="tree-canvas">
      <svg viewBox={`0 0 760 ${height}`} role="img" aria-label="Agent Runtime run tree">
        {#each rows as row}
          {@const parent = parentRow(row)}
          {#if parent}
            <path
              class="tree-edge"
              d={`M ${parent.x + 74} ${parent.y} C ${parent.x + 108} ${parent.y} ${row.x - 36} ${row.y} ${row.x - 8} ${row.y}`}
            ></path>
          {/if}
        {/each}
        {#each rows as row}
          <g
            class={nodeClass(row)}
            role="button"
            tabindex="0"
            onclick={() => onSelectRun(row.runId)}
            onkeydown={(event) => activateRun(event, row.runId)}
          >
            {#if row.tierShape === 'heavy'}
              <circle cx={row.x + 34} cy={row.y} r="30"></circle>
            {:else if row.tierShape === 'light'}
              <rect x={row.x} y={row.y - 18} width="138" height="36" rx="6"></rect>
            {:else}
              <rect x={row.x} y={row.y - 24} width="168" height="48" rx="8"></rect>
            {/if}
            <text class="node-agent" x={row.tierShape === 'heavy' ? row.x + 76 : row.x + 12} y={row.y - 5}>{row.agent}</text>
            <text class="node-meta" x={row.tierShape === 'heavy' ? row.x + 76 : row.x + 12} y={row.y + 12}>{row.tier} / {row.status}</text>
            <text class="node-id" x={row.tierShape === 'heavy' ? row.x + 76 : row.x + 12} y={row.y + 28}>{shortID(row.runId)}</text>
          </g>
        {/each}
      </svg>
    </div>
  {/if}
</section>

<style>
  .agent-runtime-tree { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); overflow: hidden; }
  .visualization-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
  .visualization-head h3 { margin: 0; }
  .visualization-head p { margin: var(--space-1) 0 0; color: var(--text-tertiary); font-size: var(--text-xs); }
  .tree-canvas { min-width: 0; overflow-x: auto; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); }
  .tree-canvas svg { display: block; min-width: 760px; width: 100%; height: auto; }
  .tree-edge { fill: none; stroke: var(--border-default); stroke-width: 2; stroke-dasharray: 5 5; opacity: 0.85; }
  .tree-node { cursor: pointer; outline: none; }
  .tree-node circle, .tree-node rect { fill: var(--surface-elevated); stroke: var(--border-default); stroke-width: 1.5; }
  .tree-node.running circle, .tree-node.running rect { stroke: var(--primary); }
  .tree-node.done circle, .tree-node.done rect { stroke: var(--success); }
  .tree-node.error circle, .tree-node.error rect { stroke: var(--error); }
  .tree-node.light rect { fill: rgba(255, 255, 255, 0.03); }
  .tree-node:hover circle, .tree-node:hover rect,
  .tree-node:focus circle, .tree-node:focus rect { stroke-width: 2.5; filter: brightness(1.12); }
  .node-agent { fill: var(--text-primary); font-family: var(--font-mono); font-size: 12px; font-weight: 600; }
  .node-meta { fill: var(--text-secondary); font-family: var(--font-mono); font-size: 10px; }
  .node-id { fill: var(--text-ghost); font-family: var(--font-mono); font-size: 9px; }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
</style>
