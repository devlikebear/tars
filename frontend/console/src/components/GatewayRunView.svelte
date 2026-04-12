<script lang="ts">
  import { onDestroy } from 'svelte'
  import { getGatewayRun, listGatewayRuns, streamGatewayRunEvents } from '../lib/api'
  import type { ConsensusVariantRecord, GatewayRun, GatewayRunEvent } from '../lib/types'

  interface Props {
    runId?: string
    onNavigate: (path: string) => void
  }

  let { runId, onNavigate }: Props = $props()

  let runs: GatewayRun[] = $state([])
  let selectedRun: GatewayRun | null = $state(null)
  let loading = $state(false)
  let error = $state('')
  let streamError = $state('')
  let events: GatewayRunEvent[] = $state([])
  let estimatedUSD = $state<number | null>(null)
  let actualUSD = $state<number | null>(null)
  let stopStream: (() => void) | null = null

  async function loadRuns() {
    loading = true
    error = ''
    try {
      runs = await listGatewayRuns()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load gateway runs'
    } finally {
      loading = false
    }
  }

  async function loadRun(id: string) {
    loading = true
    error = ''
    try {
      selectedRun = await getGatewayRun(id)
      estimatedUSD = selectedRun.consensus_budget_usd ?? null
      actualUSD = selectedRun.consensus_cost_usd ?? null
    } catch (e) {
      selectedRun = null
      error = e instanceof Error ? e.message : 'Failed to load gateway run'
    } finally {
      loading = false
    }
  }

  function startStream(id: string) {
    stopStream?.()
    streamError = ''
    events = []
    stopStream = streamGatewayRunEvents(
      id,
      (event) => {
        events = [...events, event]
        if (event.type === 'consensus_planned' && event.cost_usd_estimate != null) estimatedUSD = event.cost_usd_estimate
        if (event.type === 'consensus_finished' && event.cost_usd_actual != null) actualUSD = event.cost_usd_actual
        if (!selectedRun) return
        selectedRun = {
          ...selectedRun,
          status: event.status ?? selectedRun.status,
          response: event.response ?? selectedRun.response,
          error: event.error ?? selectedRun.error,
          resolved_alias: event.resolved_alias ?? selectedRun.resolved_alias,
          resolved_kind: event.resolved_kind ?? selectedRun.resolved_kind,
          resolved_model: event.resolved_model ?? selectedRun.resolved_model,
        }
      },
      (message) => {
        streamError = message
      },
    )
  }

  function fmtTime(value?: string): string {
    if (!value?.trim()) return '—'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function fmtUSD(value: number | null): string {
    if (value == null) return '—'
    return `$${value.toFixed(3)}`
  }

  function variantRecords(): ConsensusVariantRecord[] {
    return [...(selectedRun?.consensus_variants ?? [])].sort((a, b) => a.variant_idx - b.variant_idx)
  }

  function variantEvents(idx: number): GatewayRunEvent[] {
    return events.filter((event) => event.variant_idx === idx)
  }

  $effect(() => {
    const id = runId?.trim()
    if (id) {
      void loadRun(id)
      startStream(id)
    } else {
      stopStream?.()
      selectedRun = null
      streamError = ''
      events = []
      estimatedUSD = null
      actualUSD = null
      void loadRuns()
    }
  })

  onDestroy(() => stopStream?.())
</script>

<div class="gateway-view">
  <div class="gateway-header">
    <div>
      <div class="gateway-title">Gateway Runs</div>
      <div class="gateway-subtitle">Inspect recent subagent executions and live run events.</div>
    </div>
    {#if runId}
      <button class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/gateway')}>Back</button>
    {:else}
      <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
    {/if}
  </div>

  {#if !runId}
    {#if error}
      <div class="error-banner">{error}</div>
    {/if}
    <div class="gateway-list">
      {#if runs.length === 0 && !loading}
        <div class="gateway-empty">No gateway runs yet.</div>
      {:else}
        {#each runs as run}
          <button class="gateway-row" onclick={() => onNavigate(`/console/gateway/runs/${encodeURIComponent(run.run_id)}`)}>
            <div class="row-main">
              <span class="row-id">{run.run_id}</span>
              <span class="row-agent">{run.agent || 'default'}</span>
              <span class="row-status">{run.status}</span>
              {#if run.consensus_mode}<span class="row-mode">{run.consensus_mode}</span>{/if}
            </div>
            <div class="row-meta">
              {#if run.tier}<span>{run.tier}</span>{/if}
              {#if run.resolved_alias}<span>{run.resolved_alias}</span>{/if}
              {#if run.created_at}<span>{fmtTime(run.created_at)}</span>{/if}
            </div>
          </button>
        {/each}
      {/if}
    </div>
  {:else if loading && !selectedRun}
    <div class="gateway-empty">Loading gateway run...</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if selectedRun}
    <div class="gateway-detail">
      <div class="detail-card">
        <div class="detail-title">{selectedRun.run_id}</div>
        <div class="detail-grid">
          <div><span class="label">Agent</span><span>{selectedRun.agent || 'default'}</span></div>
          <div><span class="label">Status</span><span>{selectedRun.status}</span></div>
          <div><span class="label">Tier</span><span>{selectedRun.tier || '—'}</span></div>
          <div><span class="label">Alias</span><span>{selectedRun.resolved_alias || '—'}</span></div>
          <div><span class="label">Kind</span><span>{selectedRun.resolved_kind || '—'}</span></div>
          <div><span class="label">Model</span><span>{selectedRun.resolved_model || '—'}</span></div>
          <div><span class="label">Created</span><span>{fmtTime(selectedRun.created_at)}</span></div>
          <div><span class="label">Completed</span><span>{fmtTime(selectedRun.completed_at)}</span></div>
          <div><span class="label">Est Cost</span><span>{fmtUSD(estimatedUSD)}</span></div>
          <div><span class="label">Actual Cost</span><span>{fmtUSD(actualUSD)}</span></div>
        </div>
      </div>

      <div class="detail-columns">
        <section class="detail-panel">
          <h3>Prompt</h3>
          <pre>{selectedRun.prompt || '(none)'}</pre>
        </section>
        <section class="detail-panel">
          <h3>Response</h3>
          <pre>{selectedRun.response || selectedRun.error || '(waiting)'}</pre>
        </section>
      </div>

      {#if variantRecords().length > 0}
        <section class="detail-panel">
          <h3>Consensus Variants</h3>
          <div class="variants-grid">
            {#each variantRecords() as variant}
              <article class="variant-card">
                <div class="variant-head">
                  <strong>#{variant.variant_idx + 1}</strong>
                  <span>{variant.alias || 'variant'}</span>
                  <span>{variant.model || '—'}</span>
                </div>
                <div class="row-meta">
                  {#if variant.kind}<span>{variant.kind}</span>{/if}
                  {#if variant.status}<span>{variant.status}</span>{/if}
                  {#if variant.cost_usd != null}<span>{fmtUSD(variant.cost_usd)}</span>{/if}
                </div>
                <pre>{variant.response || variant.error || '(waiting)'}</pre>
                {#if variantEvents(variant.variant_idx).length > 0}
                  <div class="event-log">
                    {#each variantEvents(variant.variant_idx) as event}
                      <div class="event-row">
                        <span class="event-type">{event.type}</span>
                        <span class="event-body">{fmtTime(event.timestamp)}</span>
                      </div>
                    {/each}
                  </div>
                {/if}
              </article>
            {/each}
          </div>
        </section>
      {/if}

      <section class="detail-panel">
        <h3>Run Events</h3>
        {#if streamError}
          <div class="gateway-empty">{streamError}</div>
        {/if}
        {#if events.length === 0}
          <div class="gateway-empty">No live events received.</div>
        {:else}
          <div class="event-log">
            {#each events as event}
              <div class="event-row">
                <span class="event-type">{event.type}</span>
                <span class="event-body">{event.message || event.status || event.error || event.response || event.resolved_alias || fmtTime(event.timestamp)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .gateway-view { display: flex; flex-direction: column; gap: var(--space-4); }
  .gateway-header { display: flex; justify-content: space-between; gap: var(--space-4); align-items: flex-start; }
  .gateway-title { font-family: var(--font-display); font-size: var(--text-xl); color: var(--text-primary); }
  .gateway-subtitle { color: var(--text-ghost); font-size: var(--text-sm); }
  .gateway-list, .gateway-detail { display: flex; flex-direction: column; gap: var(--space-3); }
  .gateway-row, .detail-card, .detail-panel, .variant-card { text-align: left; border: 1px solid var(--border-subtle); background: var(--bg-surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .row-main, .row-meta, .variant-head { display: flex; gap: var(--space-3); flex-wrap: wrap; align-items: center; }
  .row-id, .detail-title, .event-type { font-family: var(--font-mono); }
  .row-agent, .row-status, .row-mode, .row-meta, .event-body { color: var(--text-secondary); font-size: var(--text-sm); }
  .detail-title { color: var(--text-primary); margin-bottom: var(--space-3); }
  .detail-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
  .detail-grid div { display: flex; flex-direction: column; gap: 2px; }
  .label { font-size: 10px; text-transform: uppercase; color: var(--text-ghost); font-family: var(--font-mono); }
  .detail-columns, .variants-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-3); }
  .variants-grid { grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
  pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); background: var(--bg-elevated); padding: var(--space-3); border-radius: var(--radius-md); }
  .event-log { display: flex; flex-direction: column; gap: var(--space-2); }
  .event-row { display: flex; gap: var(--space-3); align-items: flex-start; border-top: 1px solid var(--border-subtle); padding-top: var(--space-2); }
  .event-type { min-width: 140px; color: var(--accent); font-size: var(--text-xs); }
  .gateway-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
  @media (max-width: 768px) { .variants-grid { grid-template-columns: 1fr; } }
</style>
