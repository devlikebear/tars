<script lang="ts">
  import type { AgentRuntimeRun, ConsensusVariantRecord } from '../lib/types'

  interface Props {
    run: AgentRuntimeRun | null
  }

  type FlowMode = 'cost' | 'tokens'
  type CostFlowRow = {
    id: string
    label: string
    detail: string
    tier: string
    model: string
    value: number
    cost: number | null
    tokens: number
    color: string
    y: number
    width: number
  }

  let { run }: Props = $props()
  let mode: FlowMode = $state('cost')

  const tierColors: Record<string, string> = {
    heavy: 'var(--error)',
    standard: 'var(--info)',
    light: 'var(--success)',
  }

  let costFlowRuns = $derived(run ? [run] : [])
  let flowRows = $derived.by<CostFlowRow[]>(() => buildCostFlowRows(run, mode))
  let flowTotal = $derived.by<number>(() => flowRows.reduce((total, row) => total + row.value, 0))
  let flowHeight = $derived(Math.max(190, 112 + flowRows.length * 44))
  let hasCostFlow = $derived(flowRows.length > 0)

  function buildCostFlowRows(sourceRun: AgentRuntimeRun | null, valueMode: FlowMode): CostFlowRow[] {
    if (!sourceRun) return []
    const variants = [...(sourceRun.consensus_variants ?? [])].sort((a, b) => a.variant_idx - b.variant_idx)
    const rows = variants
      .map((variant) => rowFromVariant(sourceRun, variant, valueMode))
      .filter((row): row is CostFlowRow => row != null)
    if (rows.length > 0) return scaleRows(rows)

    const cost = runCostUSD(sourceRun)
    const tokens = 0
    const fallbackValue = valueMode === 'cost' ? cost ?? 0 : tokens
    if (fallbackValue <= 0) return []
    return scaleRows([{
      id: sourceRun.run_id,
      label: sourceRun.agent || 'agent',
      detail: sourceRun.resolved_model || sourceRun.resolved_alias || 'run total',
      tier: sourceRun.tier || 'default',
      model: sourceRun.resolved_model || '',
      value: fallbackValue,
      cost,
      tokens,
      color: tierColor(sourceRun.tier),
      y: 0,
      width: 0,
    }])
  }

  function rowFromVariant(sourceRun: AgentRuntimeRun, variant: ConsensusVariantRecord, valueMode: FlowMode): CostFlowRow | null {
    const cost = variant.cost_usd ?? null
    const tokens = (variant.tokens_in ?? 0) + (variant.tokens_out ?? 0)
    const value = valueMode === 'cost' ? cost ?? 0 : tokens
    if (value <= 0) return null
    const tier = sourceRun.tier || 'default'
    return {
      id: `${sourceRun.run_id}-${variant.variant_idx}`,
      label: variant.alias || `Variant ${variant.variant_idx + 1}`,
      detail: variant.model || variant.kind || 'variant',
      tier,
      model: variant.model || '',
      value,
      cost,
      tokens,
      color: tierColor(tier),
      y: 0,
      width: 0,
    }
  }

  function scaleRows(rows: CostFlowRow[]): CostFlowRow[] {
    const maxValue = Math.max(1, ...rows.map((row) => row.value))
    return rows.map((row, idx) => ({
      ...row,
      y: 74 + idx * 44,
      width: 4 + Math.round((row.value / maxValue) * 18),
    }))
  }

  function tierColor(tier?: string): string {
    return tierColors[(tier || '').toLowerCase()] ?? 'var(--primary)'
  }

  function runCostUSD(sourceRun: AgentRuntimeRun): number | null {
    if (sourceRun.consensus_cost_usd != null) return sourceRun.consensus_cost_usd
    const costs = (sourceRun.consensus_variants ?? [])
      .map((variant) => variant.cost_usd)
      .filter((cost): cost is number => cost != null)
    if (costs.length === 0) return null
    return costs.reduce((total, cost) => total + cost, 0)
  }

  function valueLabel(value: number): string {
    if (mode === 'cost') return `$${value.toFixed(3)}`
    return `${value.toLocaleString()} tokens`
  }

  function fmtUSD(value: number | null | undefined): string {
    if (value == null) return '—'
    return `$${value.toFixed(3)}`
  }

  function fmtTokens(value: number): string {
    if (value <= 0) return '—'
    return value.toLocaleString()
  }

  function planLabel(sourceRun: AgentRuntimeRun | null): string {
    if (!sourceRun) return 'Run'
    return sourceRun.root_run_id || sourceRun.parent_run_id ? `Plan ${shortID(sourceRun.root_run_id || sourceRun.parent_run_id)}` : `Run ${shortID(sourceRun.run_id)}`
  }

  function shortID(value?: string): string {
    const text = value?.trim()
    if (!text) return '—'
    return text.length > 12 ? `${text.slice(0, 12)}...` : text
  }
</script>

<section class="detail-panel cost-flow-panel" aria-label="Agent Runtime token and cost flow">
  <div class="cost-flow-head">
    <div>
      <h3>Cost Flow</h3>
      <p>{planLabel(run)} → {run?.agent || 'agent'} → variants</p>
    </div>
    <div class="cost-flow-controls" aria-label="Cost flow mode">
      <button type="button" class:active={mode === 'cost'} onclick={() => (mode = 'cost')}>Actual cost</button>
      <button type="button" class:active={mode === 'tokens'} onclick={() => (mode = 'tokens')}>Tokens</button>
    </div>
  </div>

  {#if !hasCostFlow}
    <div class="agentruntime-empty">No token or cost data recorded for this run yet.</div>
  {:else}
    <div class="cost-flow-summary">
      <div><span>Loaded runs</span><strong>{costFlowRuns.length}</strong></div>
      <div><span>{mode === 'cost' ? 'Actual cost' : 'Tokens'}</span><strong>{valueLabel(flowTotal)}</strong></div>
      <div><span>Budget</span><strong>{fmtUSD(run?.consensus_budget_usd)}</strong></div>
    </div>

    <div class="cost-flow-canvas">
      <svg viewBox={`0 0 720 ${flowHeight}`} role="img" aria-label="Token and cost Sankey diagram">
        <g class="flow-node">
          <rect x="20" y="54" width="132" height="42" rx="6"></rect>
          <text x="34" y="80">{planLabel(run)}</text>
        </g>
        <g class="flow-node">
          <rect x="300" y="54" width="132" height="42" rx="6"></rect>
          <text x="314" y="80">{run?.agent || 'agent'}</text>
        </g>
        {#each flowRows as row}
          <path
            class="flow-link"
            d={`M 152 75 C 222 75 230 ${row.y} 300 ${row.y}`}
            stroke={row.color}
            stroke-width={row.width}
          ></path>
          <path
            class="flow-link"
            d={`M 432 ${row.y} C 500 ${row.y} 520 ${row.y} 586 ${row.y}`}
            stroke={row.color}
            stroke-width={row.width}
          ></path>
          <g class="flow-node variant-node">
            <rect x="586" y={row.y - 21} width="112" height="42" rx="6"></rect>
            <text x="598" y={row.y - 2}>{row.label}</text>
            <text class="flow-subtext" x="598" y={row.y + 14}>{valueLabel(row.value)}</text>
          </g>
        {/each}
      </svg>
    </div>

    <div class="cost-flow-table">
      {#each flowRows as row}
        <div class="cost-flow-row">
          <span class="tier-dot" style={`background: ${row.color}`}></span>
          <strong title={row.detail}>{row.label}</strong>
          <span>{row.tier}</span>
          <span>{row.model || '—'}</span>
          <span>{fmtTokens(row.tokens)}</span>
          <span>{fmtUSD(row.cost)}</span>
        </div>
      {/each}
    </div>
  {/if}
</section>

<style>
  .cost-flow-panel { display: flex; flex-direction: column; gap: var(--space-3); overflow: hidden; }
  .cost-flow-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
  .cost-flow-head h3 { margin: 0; }
  .cost-flow-head p { margin: var(--space-1) 0 0; color: var(--text-tertiary); font-size: var(--text-xs); }
  .cost-flow-controls { display: inline-flex; gap: var(--space-1); border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); padding: 3px; }
  .cost-flow-controls button { min-height: 28px; border: 0; background: transparent; color: var(--text-tertiary); border-radius: var(--radius-sm); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .cost-flow-controls button.active { background: var(--surface-elevated); color: var(--text-primary); }
  .cost-flow-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-2); }
  .cost-flow-summary div { min-width: 0; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); padding: var(--space-2); }
  .cost-flow-summary span { display: block; color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .cost-flow-summary strong { display: block; margin-top: 2px; color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-md); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cost-flow-canvas { min-width: 0; overflow-x: auto; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); }
  .cost-flow-canvas svg { display: block; min-width: 560px; width: 100%; height: auto; }
  .flow-node rect { fill: var(--surface-elevated); stroke: var(--border-default); }
  .flow-node text { fill: var(--text-primary); font-family: var(--font-mono); font-size: 12px; }
  .flow-node .flow-subtext { fill: var(--text-tertiary); font-size: 10px; }
  .flow-link { fill: none; stroke-linecap: round; opacity: 0.55; }
  .cost-flow-table { display: flex; flex-direction: column; gap: 0; }
  .cost-flow-row { display: grid; grid-template-columns: 12px minmax(120px, 1fr) minmax(64px, 0.25fr) minmax(120px, 0.45fr) auto auto; gap: var(--space-2); align-items: center; border-top: 1px solid var(--border-subtle); padding: var(--space-2) 0; color: var(--text-secondary); font-size: var(--text-xs); }
  .cost-flow-row:first-child { border-top: 0; padding-top: 0; }
  .cost-flow-row:last-child { padding-bottom: 0; }
  .cost-flow-row strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-family: var(--font-mono); font-size: var(--text-xs); }
  .cost-flow-row span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tier-dot { width: 8px; height: 8px; border-radius: 999px; }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 768px) {
    .cost-flow-head { align-items: stretch; flex-direction: column; }
    .cost-flow-summary, .cost-flow-row { grid-template-columns: 1fr; }
    .tier-dot { display: none; }
  }
</style>
