<script lang="ts">
  import { buildAgentRuntimeGanttRows } from '../lib/agentruntime-graph'
  import type { AgentRuntimeRun } from '../lib/types'

  interface Props {
    runs: AgentRuntimeRun[]
    onSelectRun: (runID: string) => void
  }

  let { runs, onSelectRun }: Props = $props()
  let model = $derived(buildAgentRuntimeGanttRows(runs))

  function fmtTime(value: number): string {
    if (!Number.isFinite(value) || value <= 0) return '--:--'
    return new Intl.DateTimeFormat('en', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(new Date(value))
  }

  function percentStyle(left: number, width: number): string {
    return `left: ${left}%; width: ${width}%`
  }
</script>

<section class="agent-runtime-gantt" aria-label="Agent Runtime Gantt Strip">
  <div class="visualization-head">
    <div>
      <h3>Gantt Strip</h3>
      <p>{model.rows.length} runs on one timeline</p>
    </div>
    {#if model.hasTimeline}
      <div class="timeline-range">
        <span>{fmtTime(model.startMs)}</span>
        <span>{fmtTime(model.endMs)}</span>
      </div>
    {/if}
  </div>

  {#if !model.hasTimeline}
    <div class="agentruntime-empty">No timestamped runs available for Gantt visualization.</div>
  {:else}
    <div class="gantt-table">
      {#each model.rows as row}
        <button type="button" class="gantt-row" onclick={() => onSelectRun(row.runId)}>
          <span class="gantt-label">
            <strong>{row.label}</strong>
            <small>{row.tier} / {row.statusKind}</small>
          </span>
          <span class="gantt-track">
            <span class={`gantt-bar ${row.statusKind}`} style={percentStyle(row.leftPercent, row.widthPercent)}></span>
            {#each row.variants as variant}
              <span
                class={`variant-bar ${variant.statusKind}`}
                style={percentStyle(variant.leftPercent, variant.widthPercent)}
                title={`${variant.label}: ${variant.tokens} tokens`}
              ></span>
            {/each}
          </span>
          <span class="gantt-duration">{Math.max(1, Math.round(row.durationMs / 1000))}s</span>
        </button>
      {/each}
    </div>
  {/if}
</section>

<style>
  .agent-runtime-gantt { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); overflow: hidden; }
  .visualization-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
  .visualization-head h3 { margin: 0; }
  .visualization-head p { margin: var(--space-1) 0 0; color: var(--text-tertiary); font-size: var(--text-xs); }
  .timeline-range { display: flex; gap: var(--space-2); color: var(--text-ghost); font-family: var(--font-mono); font-size: var(--text-xs); }
  .gantt-table { display: flex; flex-direction: column; gap: var(--space-2); }
  .gantt-row { display: grid; grid-template-columns: minmax(130px, 0.24fr) minmax(220px, 1fr) auto; gap: var(--space-3); align-items: center; width: 100%; border: 1px solid var(--border-subtle); background: var(--surface-inset); color: inherit; border-radius: var(--radius-sm); padding: var(--space-2); cursor: pointer; }
  .gantt-row:hover { border-color: var(--primary); }
  .gantt-label { min-width: 0; display: grid; gap: 2px; text-align: left; }
  .gantt-label strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-family: var(--font-mono); font-size: var(--text-xs); }
  .gantt-label small, .gantt-duration { color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; }
  .gantt-track { position: relative; height: 34px; overflow: hidden; border: 1px solid var(--border-subtle); background: rgba(255, 255, 255, 0.03); border-radius: var(--radius-sm); }
  .gantt-bar, .variant-bar { position: absolute; border-radius: var(--radius-sm); }
  .gantt-bar { top: 9px; height: 16px; min-width: 3px; background: var(--text-tertiary); }
  .gantt-bar.running { background: var(--primary); }
  .gantt-bar.done { background: var(--success); }
  .gantt-bar.error { background: var(--error); }
  .variant-bar { bottom: 3px; height: 4px; min-width: 3px; background: var(--info); opacity: 0.9; }
  .variant-bar.error { background: var(--error); }
  .variant-bar.done { background: var(--success); }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 768px) {
    .visualization-head { align-items: stretch; flex-direction: column; }
    .gantt-row { grid-template-columns: 1fr; }
  }
</style>
