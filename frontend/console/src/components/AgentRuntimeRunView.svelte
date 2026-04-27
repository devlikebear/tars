<script lang="ts">
  import { onDestroy } from 'svelte'
  import {
    getAgentRuntimeRun,
    listAgentRuntimeRuns,
    listAgentRuntimeSubagents,
    streamAgentRuntimeRunEvents,
    updateAgentRuntimeSubagentTier,
  } from '../lib/api'
  import type {
    AgentRuntimeSubagent,
    AgentRuntimeSubagentsResponse,
    AgentRuntimeTierOption,
    ConsensusVariantRecord,
    AgentRuntimeRun,
    AgentRuntimeRunEvent,
  } from '../lib/types'

  interface Props {
    runId?: string
    tab?: 'runs' | 'subagents'
    onNavigate: (path: string) => void
  }

  let { runId, tab = 'runs', onNavigate }: Props = $props()

  let runs: AgentRuntimeRun[] = $state([])
  let selectedRun: AgentRuntimeRun | null = $state(null)
  let subagentsData = $state<AgentRuntimeSubagentsResponse | null>(null)
  let selectedSubagentName = $state('')
  let loading = $state(false)
  let error = $state('')
  let streamError = $state('')
  let events: AgentRuntimeRunEvent[] = $state([])
  let estimatedUSD = $state<number | null>(null)
  let actualUSD = $state<number | null>(null)
  let updatingTier = $state('')
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

  let activeTab = $derived(runId ? 'runs' : tab)
  let subagents = $derived<AgentRuntimeSubagent[]>(subagentsData?.agents ?? [])
  let tiers = $derived<AgentRuntimeTierOption[]>(subagentsData?.tiers ?? [])
  let selectedSubagentValue = $derived.by<AgentRuntimeSubagent | null>(() => {
    const selected = selectedSubagentName.trim()
    if (!selected) return subagents[0] ?? null
    return subagents.find((agent) => agent.name === selected) ?? subagents[0] ?? null
  })

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

  async function loadSubagents() {
    loading = true
    error = ''
    try {
      const data = await listAgentRuntimeSubagents()
      subagentsData = data
      if (!selectedSubagentName || !data.agents.some((agent) => agent.name === selectedSubagentName)) {
        selectedSubagentName = data.agents[0]?.name ?? ''
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load subagents'
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

  function tierSummary(tier: AgentRuntimeTierOption): string {
    if (tier.error?.trim()) return tier.error
    return [tier.provider_alias, tier.model].filter(Boolean).join(' / ') || 'unresolved'
  }

  function tierLabel(agent: AgentRuntimeSubagent): string {
    return agent.effective_tier || agent.default_tier || '—'
  }

  function tierSourceLabel(agent: AgentRuntimeSubagent): string {
    if (agent.tier_source === 'agent') return 'agent'
    if (agent.tier_source === 'role_default') return 'role default'
    if (agent.tier_source === 'default') return 'default'
    return 'unset'
  }

  function lastRunTime(agent: AgentRuntimeSubagent): string {
    const run = agent.last_run
    return fmtTime(run?.completed_at || run?.updated_at || run?.created_at)
  }

  function resolvedPreview(agent: AgentRuntimeSubagent): string {
    return [agent.resolved_alias, agent.resolved_model].filter(Boolean).join(' / ') || '—'
  }

  function selectSubagent(agent: AgentRuntimeSubagent) {
    selectedSubagentName = agent.name
  }

  async function updateSubagentTier(agent: AgentRuntimeSubagent, defaultTier: string) {
    const current = (agent.default_tier ?? '').trim()
    const next = defaultTier.trim()
    if (next === current) return
    updatingTier = agent.name
    error = ''
    try {
      const updated = await updateAgentRuntimeSubagentTier(agent.name, next)
      if (subagentsData) {
        subagentsData = {
          ...subagentsData,
          agents: subagentsData.agents.map((item) => (item.name === updated.name ? updated : item)),
        }
      }
      selectedSubagentName = updated.name
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update subagent tier'
    } finally {
      updatingTier = ''
    }
  }

  function handleTierChange(event: Event, agent: AgentRuntimeSubagent) {
    const select = event.currentTarget as HTMLSelectElement
    void updateSubagentTier(agent, select.value)
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
    } else if (activeTab === 'subagents') {
      stopStream?.()
      selectedRun = null
      streamError = ''
      events = []
      estimatedUSD = null
      actualUSD = null
      void loadSubagents()
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
      <div class="agentruntime-title">Agent Runtime</div>
      <div class="agentruntime-subtitle">
        {#if runId}
          Inspect live run events and resolved model metadata.
        {:else if activeTab === 'subagents'}
          Manage the active subagent catalog and LLM tier mapping.
        {:else}
          Inspect recent subagent executions and live run events.
        {/if}
      </div>
    </div>
    {#if runId}
      <button class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/agentruntime')}>Back</button>
    {:else if activeTab === 'subagents'}
      <button class="btn btn-ghost btn-sm" onclick={loadSubagents} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
    {:else}
      <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
    {/if}
  </div>

  {#if !runId}
    <div class="runtime-tabs" role="tablist" aria-label="Agent runtime views">
      <button
        type="button"
        class:active={activeTab === 'runs'}
        onclick={() => onNavigate('/console/agentruntime')}
        role="tab"
        aria-selected={activeTab === 'runs'}
      >
        Runs
      </button>
      <button
        type="button"
        class:active={activeTab === 'subagents'}
        onclick={() => onNavigate('/console/agentruntime/subagents')}
        role="tab"
        aria-selected={activeTab === 'subagents'}
      >
        Subagents
      </button>
    </div>
  {/if}

  {#if !runId && activeTab === 'runs'}
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
  {:else if !runId && activeTab === 'subagents'}
    {#if error}
      <div class="error-banner">{error}</div>
    {/if}

    <section class="subagents-summary" aria-label="LLM tier catalog">
      <div>
        <div class="eyebrow">Tier Catalog</div>
        <div class="tier-line">
          {#if tiers.length === 0}
            <span class="tier-chip missing">No tiers</span>
          {:else}
            {#each tiers as tier}
              <span class="tier-chip" class:missing={!!tier.error} title={tierSummary(tier)}>
                {tier.name}
              </span>
            {/each}
          {/if}
        </div>
      </div>
      <button class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/config')}>Manage LLM Tiers</button>
    </section>

    <div class="subagents-layout">
      <section class="subagents-list" aria-label="Subagents">
        {#if subagents.length === 0 && !loading}
          <div class="agentruntime-empty">No subagents configured.</div>
        {:else}
          {#each subagents as agent}
            <button
              class="subagent-row"
              class:selected={selectedSubagentValue?.name === agent.name}
              onclick={() => selectSubagent(agent)}
            >
              <div class="row-main">
                <span class="row-agent">{agent.name}</span>
                {#if agent.default}<span class="row-mode">default</span>{/if}
                <span class="row-status">{agent.enabled ? 'enabled' : 'disabled'}</span>
              </div>
              <div class="row-meta">
                <span class:tier-warning={agent.tier_missing}>{tierLabel(agent)}</span>
                <span>{resolvedPreview(agent)}</span>
                <span>{lastRunTime(agent)}</span>
              </div>
            </button>
          {/each}
        {/if}
      </section>

      <section class="subagent-detail" aria-label="Subagent detail">
        {#if !selectedSubagentValue}
          <div class="agentruntime-empty">Select a subagent.</div>
        {:else}
          <div class="detail-head">
            <div>
              <div class="detail-title">{selectedSubagentValue.name}</div>
              <p>{selectedSubagentValue.description || 'No description'}</p>
            </div>
            {#if selectedSubagentValue.tier_editable}
              <label class="tier-editor">
                <span>LLM Tier</span>
                <select
                  value={selectedSubagentValue.default_tier ?? ''}
                  disabled={updatingTier === selectedSubagentValue.name || tiers.length === 0}
                  onchange={(event) => handleTierChange(event, selectedSubagentValue)}
                >
                  <option value="">Inherit runtime default</option>
                  {#each tiers as tier}
                    <option value={tier.name} disabled={!!tier.error}>{tier.name}</option>
                  {/each}
                </select>
              </label>
            {:else}
              <span class="tier-chip" class:missing={selectedSubagentValue.tier_missing}>{tierLabel(selectedSubagentValue)}</span>
            {/if}
          </div>

          {#if selectedSubagentValue.tier_error}
            <div class="error-banner">{selectedSubagentValue.tier_error}</div>
          {/if}

          <div class="detail-grid">
            <div><span class="label">Source</span><span>{selectedSubagentValue.source || '—'}</span></div>
            <div><span class="label">Kind</span><span>{selectedSubagentValue.kind || '—'}</span></div>
            <div><span class="label">Tier Source</span><span>{tierSourceLabel(selectedSubagentValue)}</span></div>
            <div><span class="label">Provider</span><span>{selectedSubagentValue.resolved_alias || '—'}</span></div>
            <div><span class="label">Model</span><span>{selectedSubagentValue.resolved_model || '—'}</span></div>
            <div><span class="label">Runs</span><span>{selectedSubagentValue.run_count}</span></div>
            <div><span class="label">Policy</span><span>{selectedSubagentValue.policy_mode || 'full'}</span></div>
            <div><span class="label">Routing</span><span>{selectedSubagentValue.session_routing_mode || 'caller'}</span></div>
          </div>

          {#if selectedSubagentValue.entry}
            <section class="detail-panel">
              <h3>Entry</h3>
              <pre>{selectedSubagentValue.entry}</pre>
            </section>
          {/if}

          <section class="detail-panel">
            <h3>Tool Policy</h3>
            <div class="policy-grid">
              <div>
                <span class="label">Allow</span>
                <p>{selectedSubagentValue.tools_allow.length > 0 ? selectedSubagentValue.tools_allow.join(', ') : 'all tools'}</p>
              </div>
              <div>
                <span class="label">Deny</span>
                <p>{selectedSubagentValue.tools_deny.length > 0 ? selectedSubagentValue.tools_deny.join(', ') : 'none'}</p>
              </div>
              <div>
                <span class="label">Groups</span>
                <p>{[...selectedSubagentValue.tools_allow_groups, ...selectedSubagentValue.tools_deny_groups].join(', ') || 'none'}</p>
              </div>
              <div>
                <span class="label">Risk Max</span>
                <p>{selectedSubagentValue.tools_risk_max || '—'}</p>
              </div>
            </div>
          </section>

          <section class="detail-panel">
            <h3>Recent Runs</h3>
            {#if selectedSubagentValue.recent_runs.length === 0}
              <div class="agentruntime-empty">No runs recorded for this subagent.</div>
            {:else}
              <div class="recent-runs">
                {#each selectedSubagentValue.recent_runs as recent}
                  <button class="recent-run" onclick={() => onNavigate(`/console/agentruntime/runs/${encodeURIComponent(recent.run_id)}`)}>
                    <span class="row-id">{recent.run_id}</span>
                    <span>{recent.status}</span>
                    <span>{fmtTime(recent.completed_at || recent.updated_at || recent.created_at)}</span>
                  </button>
                {/each}
              </div>
            {/if}
          </section>
        {/if}
      </section>
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
  .runtime-tabs { display: inline-flex; gap: var(--space-1); align-self: flex-start; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); padding: 3px; }
  .runtime-tabs button { border: 0; background: transparent; color: var(--text-tertiary); border-radius: var(--radius-sm); padding: var(--space-2) var(--space-3); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .runtime-tabs button.active { background: var(--surface-elevated); color: var(--text-primary); }
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
  .subagents-summary { display: flex; justify-content: space-between; gap: var(--space-4); align-items: center; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .tier-line { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-2); }
  .tier-chip { display: inline-flex; align-items: center; max-width: 220px; min-height: 24px; border: 1px solid var(--border-subtle); background: var(--primary-muted); color: var(--primary-text); border-radius: var(--radius-sm); padding: 2px var(--space-2); font-family: var(--font-mono); font-size: var(--text-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tier-chip.missing { background: var(--warning-muted); color: var(--warning); }
  .tier-warning { color: var(--warning); }
  .tier-editor { display: flex; flex-direction: column; gap: var(--space-1); min-width: min(220px, 100%); color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .tier-editor select { width: 100%; min-height: 32px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-primary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); text-transform: none; }
  .subagents-layout { display: grid; grid-template-columns: minmax(260px, 0.45fr) minmax(0, 1fr); gap: var(--space-3); align-items: start; }
  .subagents-list, .subagent-detail { min-width: 0; display: flex; flex-direction: column; gap: var(--space-3); }
  .subagents-list { border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-2); }
  .subagent-row, .recent-run { width: 100%; text-align: left; border: 1px solid transparent; background: transparent; color: inherit; border-radius: var(--radius-sm); padding: var(--space-3); cursor: pointer; }
  .subagent-row:hover, .recent-run:hover { background: var(--surface-elevated); }
  .subagent-row.selected { border-color: var(--border-default); background: var(--surface-elevated); }
  .detail-head { display: flex; justify-content: space-between; gap: var(--space-3); align-items: flex-start; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .detail-head p { margin: var(--space-1) 0 0; color: var(--text-secondary); font-size: var(--text-sm); }
  .policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-3); }
  .policy-grid p { margin: 0; color: var(--text-secondary); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .recent-runs { display: flex; flex-direction: column; gap: var(--space-2); }
  .recent-run { display: grid; grid-template-columns: minmax(120px, 1fr) auto auto; gap: var(--space-3); align-items: center; color: var(--text-secondary); font-size: var(--text-sm); }
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
  @media (max-width: 960px) { .subagents-layout { grid-template-columns: 1fr; } }
  @media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
  @media (max-width: 768px) { .variants-grid, .prompt-grid, .policy-grid, .recent-run { grid-template-columns: 1fr; } .subagents-summary, .detail-head { align-items: stretch; flex-direction: column; } }
</style>
