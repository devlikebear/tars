<script lang="ts">
  import { onDestroy } from 'svelte'
  import { getAgentRuntimeRun, listAgentRuntimeRuns, streamAgentRuntimeRunEvents } from '../lib/api'
  import type { ConsensusVariantRecord, AgentRuntimeRun, AgentRuntimeRunEvent } from '../lib/types'

  interface Props {
    runId?: string
    onNavigate: (path: string) => void
  }

  let { runId, onNavigate }: Props = $props()

  let runs: AgentRuntimeRun[] = $state([])
  let selectedRun: AgentRuntimeRun | null = $state(null)
  let loading = $state(false)
  let error = $state('')
  let streamError = $state('')
  let events: AgentRuntimeRunEvent[] = $state([])
  let estimatedUSD = $state<number | null>(null)
  let actualUSD = $state<number | null>(null)
  let stopStream: (() => void) | null = null

  const runtimeTools = [
    { name: 'subagents_run', detail: 'Single delegated task on a selected model tier.' },
    { name: 'subagents_orchestrate', detail: 'Parallel steps split across multiple subagents.' },
    { name: 'subagents_plan', detail: 'Plan first, then launch the selected steps.' },
  ]

  const starterPrompts = [
    'Analyze this codebase in parallel with three subagents.',
    'Use subagents_plan to break this into five steps and run it.',
  ]

  async function loadRuns() {
    loading = true
    error = ''
    try {
      runs = await listAgentRuntimeRuns()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load agent runtime runs'
    } finally {
      loading = false
    }
  }

  async function loadRun(id: string) {
    loading = true
    error = ''
    try {
      selectedRun = await getAgentRuntimeRun(id)
      estimatedUSD = selectedRun.consensus_budget_usd ?? null
      actualUSD = selectedRun.consensus_cost_usd ?? null
    } catch (e) {
      selectedRun = null
      error = e instanceof Error ? e.message : 'Failed to load agent runtime run'
    } finally {
      loading = false
    }
  }

  function startStream(id: string) {
    stopStream?.()
    streamError = ''
    events = []
    stopStream = streamAgentRuntimeRunEvents(
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

  function variantEvents(idx: number): AgentRuntimeRunEvent[] {
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

<div class="agentruntime-view">
  <div class="agentruntime-header">
    <div>
      <div class="agentruntime-title">Agent Runtime Runs</div>
      <div class="agentruntime-subtitle">Inspect recent subagent executions and live run events.</div>
    </div>
    {#if runId}
      <button class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/agentruntime')}>Back</button>
    {:else}
      <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
    {/if}
  </div>

  {#if !runId}
    <section class="intro-card" aria-labelledby="agentruntime-intro-title">
      <div class="intro-copy">
        <div class="eyebrow">Background Subagents</div>
        <h2 id="agentruntime-intro-title">Agent Runtime</h2>
        <p>
          Runs appear here when chat starts delegated work in the agent runtime.
          Each record keeps the prompt, model tier, status, response, live events, and consensus cost data when available.
        </p>
      </div>
      <div class="tool-strip" aria-label="Agent runtime tool families">
        {#each runtimeTools as tool}
          <div class="tool-chip">
            <code>{tool.name}</code>
            <span>{tool.detail}</span>
          </div>
        {/each}
      </div>
    </section>

    {#if error}
      <div class="error-banner">{error}</div>
    {/if}
    <div class="agentruntime-list">
      {#if runs.length === 0 && !loading}
        <section class="empty-guide" aria-labelledby="agentruntime-empty-title">
          <div>
            <div class="eyebrow">No Runs Yet</div>
            <h3 id="agentruntime-empty-title">Start from Chat</h3>
            <p>
              Agent Runtime only records work launched by the subagent tools.
              Try one of these prompts in a chat session, then return here to inspect the run history.
            </p>
          </div>
          <div class="prompt-grid">
            {#each starterPrompts as prompt}
              <blockquote>{prompt}</blockquote>
            {/each}
          </div>
          <div class="empty-actions">
            <button class="btn btn-secondary btn-sm" onclick={() => onNavigate('/console/chat')}>Open Chat</button>
            <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
          </div>
        </section>
      {:else}
        {#each runs as run}
          <button class="agentruntime-row" onclick={() => onNavigate(`/console/agentruntime/runs/${encodeURIComponent(run.run_id)}`)}>
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
    <div class="agentruntime-empty">Loading agent runtime run...</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if selectedRun}
    <div class="agentruntime-detail">
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
          <div class="agentruntime-empty">{streamError}</div>
        {/if}
        {#if events.length === 0}
          <div class="agentruntime-empty">No live events received.</div>
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
  .agentruntime-view { display: flex; flex-direction: column; gap: var(--space-4); }
  .agentruntime-header { display: flex; justify-content: space-between; gap: var(--space-4); align-items: flex-start; }
  .agentruntime-title { font-family: var(--font-display); font-size: var(--text-xl); color: var(--text-primary); }
  .agentruntime-subtitle { color: var(--text-ghost); font-size: var(--text-sm); }
  .intro-card, .empty-guide { border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-lg); padding: var(--space-5); }
  .intro-card { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(280px, 0.8fr); gap: var(--space-5); align-items: start; }
  .intro-copy { display: flex; flex-direction: column; gap: var(--space-2); }
  .intro-copy h2, .empty-guide h3 { margin: 0; }
  .intro-copy p, .empty-guide p { color: var(--text-secondary); font-size: var(--text-sm); max-width: 720px; }
  .eyebrow { font-family: var(--font-display); font-size: var(--text-xs); font-weight: 500; letter-spacing: 0.04em; color: var(--primary-text); text-transform: uppercase; }
  .tool-strip { display: flex; flex-direction: column; gap: var(--space-2); }
  .tool-chip { display: grid; gap: 2px; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); padding: var(--space-3); }
  .tool-chip code { color: var(--text-primary); font-size: var(--text-xs); }
  .tool-chip span { color: var(--text-tertiary); font-size: var(--text-xs); }
  .agentruntime-list, .agentruntime-detail { display: flex; flex-direction: column; gap: var(--space-3); }
  .agentruntime-row, .detail-card, .detail-panel, .variant-card { text-align: left; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .empty-guide { display: flex; flex-direction: column; gap: var(--space-4); }
  .prompt-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-3); }
  blockquote { margin: 0; border-left: 2px solid var(--primary); background: var(--surface-inset); border-radius: var(--radius-md); padding: var(--space-3); color: var(--text-secondary); font-size: var(--text-sm); }
  .empty-actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .row-main, .row-meta, .variant-head { display: flex; gap: var(--space-3); flex-wrap: wrap; align-items: center; }
  .row-id, .detail-title, .event-type { font-family: var(--font-mono); }
  .row-agent, .row-status, .row-mode, .row-meta, .event-body { color: var(--text-secondary); font-size: var(--text-sm); }
  .detail-title { color: var(--text-primary); margin-bottom: var(--space-3); }
  .detail-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
  .detail-grid div { display: flex; flex-direction: column; gap: 2px; }
  .label { font-size: 10px; text-transform: uppercase; color: var(--text-ghost); font-family: var(--font-mono); }
  .detail-columns, .variants-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-3); }
  .variants-grid { grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
  pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); background: var(--surface-elevated); padding: var(--space-3); border-radius: var(--radius-md); }
  .event-log { display: flex; flex-direction: column; gap: var(--space-2); }
  .event-row { display: flex; gap: var(--space-3); align-items: flex-start; border-top: 1px solid var(--border-subtle); padding-top: var(--space-2); }
  .event-type { min-width: 140px; color: var(--primary); font-size: var(--text-xs); }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 960px) { .intro-card { grid-template-columns: 1fr; } }
  @media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
  @media (max-width: 768px) { .variants-grid, .prompt-grid { grid-template-columns: 1fr; } }
</style>
