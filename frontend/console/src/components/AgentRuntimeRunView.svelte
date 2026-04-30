<script lang="ts">
  import { onDestroy } from 'svelte'
  import AgentRuntimeCostFlow from './AgentRuntimeCostFlow.svelte'
  import AgentRuntimeGantt from './AgentRuntimeGantt.svelte'
  import AgentRuntimeReplay from './AgentRuntimeReplay.svelte'
  import AgentRuntimeTree from './AgentRuntimeTree.svelte'
  import {
    applyAgentRuntimeSubagentDraft,
    archiveAgentRuntimeSubagent,
    draftAgentRuntimeSubagent,
    getAgentRuntimeRun,
    listAgentRuntimeRuns,
    listAgentRuntimeSubagents,
    streamAgentRuntimeRunEvents,
    updateAgentRuntimeSubagentTier,
  } from '../lib/api'
  import type {
    AgentRuntimeSubagent,
    AgentRuntimeSubagentDraft,
    AgentRuntimeSubagentDraftResponse,
    AgentRuntimeSubagentsResponse,
    AgentRuntimeTierOption,
    ConsensusVariantRecord,
    FileAttentionSummary,
    AgentRuntimeRun,
    AgentRuntimeRunEvent,
  } from '../lib/types'

  interface Props {
    runId?: string
    tab?: 'runs' | 'subagents'
    onNavigate: (path: string) => void
  }

  type RunStatusFilter = 'all' | 'running' | 'done' | 'failed'
  type RunTimeRange = '24h' | '7d' | 'all'
  type RunViewMode = 'list' | 'tree' | 'gantt'
  type PlanCostRow = {
    key: string
    label: string
    total: number
    runs: number
  }
  type FileAttentionAction = 'read' | 'edit' | 'both'

  let { runId, tab = 'runs', onNavigate }: Props = $props()

  let runs: AgentRuntimeRun[] = $state([])
  let summaryRuns: AgentRuntimeRun[] = $state([])
  let selectedRun: AgentRuntimeRun | null = $state(null)
  let subagentsData = $state<AgentRuntimeSubagentsResponse | null>(null)
  let selectedSubagentName = $state('')
  let runStatusFilter: RunStatusFilter = $state('all')
  let runTimeRange: RunTimeRange = $state('all')
  let runViewMode: RunViewMode = $state('list')
  let runSearchInput = $state('')
  let loading = $state(false)
  let error = $state('')
  let streamError = $state('')
  let events: AgentRuntimeRunEvent[] = $state([])
  let estimatedUSD = $state<number | null>(null)
  let actualUSD = $state<number | null>(null)
  let updatingTier = $state('')
  let builderMode: 'create' | 'edit' = $state('create')
  let builderOpen = $state(false)
  let builderRequest = $state('')
  let builderBaseName = $state('')
  let builderTier = $state('')
  let builderBusy = $state(false)
  let builderApplying = $state(false)
  let builderResponse: AgentRuntimeSubagentDraftResponse | null = $state(null)
  let archiveConfirmName = $state('')
  let archiveBusy = $state(false)
  let stopStream: (() => void) | null = null

  const runtimeTools = [
    { name: 'subagents_run', detail: 'Single delegated task on a selected model tier.' },
    { name: 'subagents_orchestrate', detail: 'Advanced opt-in staged flow for dependent steps.' },
    { name: 'subagents_plan', detail: 'Advanced opt-in planner for staged flows.' },
  ]

  const starterPrompts = [
    'Analyze this codebase in parallel with three subagents.',
    'Ask two subagents to inspect the frontend and backend separately.',
  ]

  const runStatusOptions: { value: RunStatusFilter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'done', label: 'Done' },
    { value: 'failed', label: 'Failed' },
  ]

  const runTimeRangeOptions: { value: RunTimeRange; label: string }[] = [
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: 'all', label: 'All' },
  ]

  const runViewModeOptions: { value: RunViewMode; label: string }[] = [
    { value: 'list', label: 'List' },
    { value: 'tree', label: 'Tree' },
    { value: 'gantt', label: 'Gantt' },
  ]

  let activeTab = $derived(runId ? 'runs' : tab)
  let subagents = $derived<AgentRuntimeSubagent[]>(subagentsData?.agents ?? [])
  let tiers = $derived<AgentRuntimeTierOption[]>(subagentsData?.tiers ?? [])
  let selectedSubagentValue = $derived.by<AgentRuntimeSubagent | null>(() => {
    const selected = selectedSubagentName.trim()
    if (!selected) return subagents[0] ?? null
    return subagents.find((agent) => agent.name === selected) ?? subagents[0] ?? null
  })
  let todayCostUSD = $derived.by<number | null>(() => {
    const start = new Date()
    start.setHours(0, 0, 0, 0)
    return sumRunCosts(summaryRuns.filter((run) => runTimestamp(run) >= start.getTime()))
  })
  let sevenDayCostUSD = $derived.by<number | null>(() => {
    const cutoff = Date.now() - 7 * 24 * 60 * 60 * 1000
    return sumRunCosts(summaryRuns.filter((run) => runTimestamp(run) >= cutoff))
  })
  let planCostRows = $derived.by<PlanCostRow[]>(() => groupedPlanCosts(summaryRuns))
  let fileAttentionRows = $derived.by<FileAttentionSummary[]>(() => {
    return [...(selectedRun?.file_attention ?? [])]
      .sort((a, b) => (b.total ?? 0) - (a.total ?? 0) || a.path.localeCompare(b.path))
      .slice(0, 24)
  })
  let fileAttentionMax = $derived.by<number>(() => {
    return Math.max(1, ...fileAttentionRows.map((row) => row.total ?? 0))
  })
  let costFlowRuns = $derived.by<AgentRuntimeRun[]>(() => selectedRun ? [selectedRun] : [])
  let replayEvents = $derived.by<AgentRuntimeRunEvent[]>(() => events)

  async function loadRuns() {
    loading = true
    error = ''
    try {
      const search = runSearchInput.trim()
      const [visibleRuns, costRuns] = await Promise.all([
        listAgentRuntimeRuns({
          limit: 100,
          status: runStatusFilter,
          since: runTimeRange,
          search,
        }),
        listAgentRuntimeRuns({
          limit: 200,
          status: runStatusFilter,
          since: 'all',
          search,
        }),
      ])
      runs = visibleRuns
      summaryRuns = costRuns
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
        const nextRun = {
          ...selectedRun,
          status: event.status ?? selectedRun.status,
          response: event.response ?? selectedRun.response,
          error: event.error ?? selectedRun.error,
          resolved_alias: event.resolved_alias ?? selectedRun.resolved_alias,
          resolved_kind: event.resolved_kind ?? selectedRun.resolved_kind,
          resolved_model: event.resolved_model ?? selectedRun.resolved_model,
        }
        selectedRun = event.type === 'tool.call' ? applyFileAttentionEvent(nextRun, event) : nextRun
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

  function runTimestamp(run: AgentRuntimeRun): number {
    for (const value of [run.created_at, run.started_at, run.updated_at, run.completed_at]) {
      if (!value?.trim()) continue
      const parsed = new Date(value).getTime()
      if (!Number.isNaN(parsed)) return parsed
    }
    return 0
  }

  function shortID(value?: string): string {
    const text = value?.trim()
    if (!text) return '—'
    return text.length > 12 ? `${text.slice(0, 12)}…` : text
  }

  function runCostUSD(run: AgentRuntimeRun): number | null {
    if (run.consensus_cost_usd != null) return run.consensus_cost_usd
    const variantCosts = (run.consensus_variants ?? [])
      .map((variant) => variant.cost_usd)
      .filter((cost): cost is number => cost != null)
    if (variantCosts.length === 0) return null
    return variantCosts.reduce((total, cost) => total + cost, 0)
  }

  function sumRunCosts(sourceRuns: AgentRuntimeRun[]): number | null {
    let total = 0
    let found = false
    for (const run of sourceRuns) {
      const cost = runCostUSD(run)
      if (cost == null) continue
      total += cost
      found = true
    }
    return found ? total : null
  }

  function groupedPlanCosts(sourceRuns: AgentRuntimeRun[]): PlanCostRow[] {
    const groups = new Map<string, PlanCostRow>()
    for (const run of sourceRuns) {
      const cost = runCostUSD(run)
      if (cost == null) continue
      const key = run.root_run_id || run.parent_run_id || run.run_id
      const label = run.root_run_id || run.parent_run_id
        ? `Plan ${shortID(key)}`
        : run.prompt?.trim() || `Run ${shortID(run.run_id)}`
      const existing = groups.get(key) ?? { key, label, total: 0, runs: 0 }
      existing.total += cost
      existing.runs += 1
      groups.set(key, existing)
    }
    return [...groups.values()]
      .sort((a, b) => b.total - a.total)
      .slice(0, 3)
  }

  function fileAttentionAction(row: FileAttentionSummary): FileAttentionAction {
    const reads = row.reads ?? 0
    const edits = row.edits ?? 0
    if (reads > 0 && edits > 0) return 'both'
    if (edits > 0) return 'edit'
    return 'read'
  }

  function fileAttentionIntensity(row: FileAttentionSummary): number {
    return Math.max(8, Math.round(((row.total ?? 0) / fileAttentionMax) * 100))
  }

  function fileAttentionOpsTotal(): number {
    return selectedRun?.file_ops_total ?? fileAttentionRows.reduce((total, row) => total + (row.total ?? 0), 0)
  }

  function normalizedSparkline(values?: number[]): number[] {
    if (!values?.length) return [0]
    return values
  }

  function sparklineHeight(value: number, values?: number[]): number {
    const max = Math.max(1, ...(values ?? [0]))
    return Math.max(10, Math.round((value / max) * 100))
  }

  function applyFileAttentionEvent(run: AgentRuntimeRun, event: AgentRuntimeRunEvent): AgentRuntimeRun {
    const path = event.path?.trim()
    if (!path) return run
    const rows = [...(run.file_attention ?? [])].map((row) => ({ ...row, sparkline: [...(row.sparkline ?? [])] }))
    let row = rows.find((item) => item.path === path)
    if (!row) {
      row = { path, total: 0, sparkline: [] }
      rows.push(row)
    }
    row.total = (row.total ?? 0) + 1
    if (event.action === 'edit') {
      row.edits = (row.edits ?? 0) + 1
    } else {
      row.reads = (row.reads ?? 0) + 1
      if (event.tool_name === 'list_dir') row.lists = (row.lists ?? 0) + 1
    }
    if (event.tool_name === 'write_file' || event.tool_name === 'write') row.writes = (row.writes ?? 0) + 1
    row.last_at = event.timestamp ?? row.last_at
    if (!row.first_at) row.first_at = row.last_at
    const sparkline = row.sparkline ?? []
    if (sparkline.length < 12) sparkline.push(1)
    else sparkline[sparkline.length - 1] = (sparkline[sparkline.length - 1] ?? 0) + 1
    row.sparkline = sparkline
    rows.sort((a, b) => (b.total ?? 0) - (a.total ?? 0) || a.path.localeCompare(b.path))
    return {
      ...run,
      file_attention: rows,
      file_ops_total: (run.file_ops_total ?? 0) + 1,
    }
  }

  function runFiltersActive(): boolean {
    return runStatusFilter !== 'all' || runTimeRange !== 'all' || runSearchInput.trim() !== ''
  }

  function setRunStatusFilter(status: RunStatusFilter) {
    runStatusFilter = status
    void loadRuns()
  }

  function setRunTimeRange(range: RunTimeRange) {
    runTimeRange = range
    void loadRuns()
  }

  function setRunViewMode(mode: RunViewMode) {
    runViewMode = mode
  }

  function openRunDetail(id: string) {
    onNavigate(`/console/agentruntime/runs/${encodeURIComponent(id)}`)
  }

  function handleRunSearchKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      void loadRuns()
    }
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

  function defaultBuilderTier(): string {
    return selectedSubagentValue?.default_tier || subagentsData?.agentruntime_default_tier || subagentsData?.default_tier || tiers[0]?.name || ''
  }

  function openCreateBuilder() {
    builderOpen = true
    builderMode = 'create'
    builderBaseName = ''
    builderRequest = ''
    builderTier = defaultBuilderTier()
    builderResponse = null
    archiveConfirmName = ''
  }

  function openEditBuilder(agent: AgentRuntimeSubagent) {
    builderOpen = true
    builderMode = 'edit'
    builderBaseName = agent.name
    builderRequest = ''
    builderTier = agent.default_tier || defaultBuilderTier()
    builderResponse = null
    archiveConfirmName = ''
  }

  function closeBuilder() {
    builderOpen = false
    builderMode = 'create'
    builderBaseName = ''
    builderRequest = ''
    builderTier = ''
    builderResponse = null
  }

  async function requestSubagentDraft() {
    const request = builderRequest.trim()
    if (!request) {
      error = 'Describe what the subagent should do.'
      return
    }
    builderBusy = true
    error = ''
    try {
      builderResponse = await draftAgentRuntimeSubagent({
        mode: builderMode,
        request,
        base_name: builderMode === 'edit' ? builderBaseName : undefined,
        default_tier: builderTier || defaultBuilderTier(),
      })
      builderTier = builderResponse.draft.default_tier || builderTier
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to draft subagent'
    } finally {
      builderBusy = false
    }
  }

  function updateDraftField(field: keyof AgentRuntimeSubagentDraft, value: string) {
    if (!builderResponse) return
    builderResponse = {
      ...builderResponse,
      draft: { ...builderResponse.draft, [field]: value },
    }
  }

  function updateDraftList(field: 'tools_allow' | 'tools_deny', value: string) {
    if (!builderResponse) return
    builderResponse = {
      ...builderResponse,
      draft: {
        ...builderResponse.draft,
        [field]: value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean),
      },
    }
  }

  async function applyDraft() {
    if (!builderResponse) return
    builderApplying = true
    error = ''
    try {
      const updated = await applyAgentRuntimeSubagentDraft(builderResponse.draft)
      await loadSubagents()
      selectedSubagentName = updated.name
      closeBuilder()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to apply subagent draft'
    } finally {
      builderApplying = false
    }
  }

  async function archiveSubagent(agent: AgentRuntimeSubagent) {
    if (archiveConfirmName !== agent.name) {
      archiveConfirmName = agent.name
      return
    }
    archiveBusy = true
    error = ''
    try {
      await archiveAgentRuntimeSubagent(agent.name, true)
      archiveConfirmName = ''
      await loadSubagents()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to archive subagent'
    } finally {
      archiveBusy = false
    }
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
      <div class="header-actions">
        <button class="btn btn-primary btn-sm" onclick={openCreateBuilder}>New Subagent</button>
        <button class="btn btn-ghost btn-sm" onclick={loadSubagents} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
      </div>
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
          Each record keeps the prompt, model tier, status, response, live events, and advanced cost data when available.
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

    <section class="run-controls" aria-label="Agent Runtime run filters">
      <div class="filter-group">
        <span class="filter-label">Status</span>
        <div class="filter-chip-row">
          {#each runStatusOptions as option}
            <button
              type="button"
              class="filter-chip"
              class:active={runStatusFilter === option.value}
              onclick={() => setRunStatusFilter(option.value)}
            >
              {option.label}
            </button>
          {/each}
        </div>
      </div>
      <div class="filter-group">
        <span class="filter-label">Time range</span>
        <div class="filter-chip-row">
          {#each runTimeRangeOptions as option}
            <button
              type="button"
              class="filter-chip"
              class:active={runTimeRange === option.value}
              onclick={() => setRunTimeRange(option.value)}
            >
              {option.label}
            </button>
          {/each}
        </div>
      </div>
      <label class="run-search-field">
        <span>Search prompt</span>
        <input
          bind:value={runSearchInput}
          onkeydown={handleRunSearchKeydown}
          placeholder="Prompt, run id, agent, session"
        />
      </label>
      <button class="btn btn-primary btn-sm" type="button" onclick={loadRuns} disabled={loading}>
        {loading ? 'Loading...' : 'Apply'}
      </button>
    </section>

    <section class="run-view-mode" aria-label="Agent Runtime visualization mode">
      {#each runViewModeOptions as option}
        <button
          type="button"
          class:active={runViewMode === option.value}
          onclick={() => setRunViewMode(option.value)}
        >
          {option.label}
        </button>
      {/each}
    </section>

    <section class="cost-summary-grid" aria-label="Agent Runtime cost summary">
      <div class="cost-summary-card">
        <span>Today</span>
        <strong>{fmtUSD(todayCostUSD)}</strong>
        <small>Loaded run costs</small>
      </div>
      <div class="cost-summary-card">
        <span>7d</span>
        <strong>{fmtUSD(sevenDayCostUSD)}</strong>
        <small>Loaded run costs</small>
      </div>
      <div class="cost-summary-card plan-summary">
        <span>Plan totals</span>
        {#if planCostRows.length === 0}
          <small>No advanced cost data yet</small>
        {:else}
          <div class="plan-cost-list">
            {#each planCostRows as row}
              <div class="plan-cost-row">
                <strong title={row.key}>{row.label}</strong>
                <span>{fmtUSD(row.total)} · {row.runs} {row.runs === 1 ? 'run' : 'runs'}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    {#if error}
      <div class="error-banner">{error}</div>
    {/if}
    {#if runViewMode === 'tree' && runs.length > 0}
      <AgentRuntimeTree {runs} onSelectRun={openRunDetail} />
    {:else if runViewMode === 'gantt' && runs.length > 0}
      <AgentRuntimeGantt {runs} onSelectRun={openRunDetail} />
    {:else}
    <div class="agentruntime-list">
      {#if runs.length === 0 && !loading}
        {#if runFiltersActive()}
          <section class="empty-guide" aria-labelledby="agentruntime-empty-title">
            <div>
              <div class="eyebrow">No Matching Runs</div>
              <h3 id="agentruntime-empty-title">Adjust filters</h3>
              <p>No Agent Runtime runs match the current status, time range, and search query.</p>
            </div>
            <div class="empty-actions">
              <button
                class="btn btn-secondary btn-sm"
                onclick={() => {
                  runStatusFilter = 'all'
                  runTimeRange = 'all'
                  runSearchInput = ''
                  void loadRuns()
                }}
              >
                Clear Filters
              </button>
              <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
            </div>
          </section>
        {:else}
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
        {/if}
      {:else}
        {#each runs as run}
          <article class="agentruntime-row">
            <button class="run-open-button" type="button" onclick={() => openRunDetail(run.run_id)}>
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
                {#if runCostUSD(run) != null}<span>{fmtUSD(runCostUSD(run))}</span>{/if}
              </div>
              {#if run.prompt}
                <p class="row-prompt">{run.prompt}</p>
              {/if}
            </button>
            {#if run.session_id}
              <button
                class="session-link"
                type="button"
                title={`Open chat session ${run.session_id}`}
                onclick={() => onNavigate(`/console/chat/${encodeURIComponent(run.session_id || '')}`)}
              >
                Started from session: {shortID(run.session_id)}
              </button>
            {/if}
          </article>
        {/each}
      {/if}
    </div>
    {/if}
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

    {#if builderOpen}
      <section class="builder-panel" aria-label="Subagent builder">
        <div class="builder-head">
          <div>
            <div class="eyebrow">{builderMode === 'edit' ? 'Edit with LLM' : 'Create with LLM'}</div>
            <h3>{builderMode === 'edit' ? builderBaseName : 'New subagent'}</h3>
          </div>
          <button class="btn btn-ghost btn-sm" onclick={closeBuilder}>Close</button>
        </div>
        <div class="builder-form">
          <label>
            <span>Request</span>
            <textarea
              bind:value={builderRequest}
              placeholder={builderMode === 'edit' ? 'Make this subagent focus on frontend accessibility.' : 'Create a frontend reviewer agent.'}
            ></textarea>
          </label>
          <label>
            <span>Default Tier</span>
            <select bind:value={builderTier}>
              {#each tiers as tier}
                <option value={tier.name} disabled={!!tier.error}>{tier.name}</option>
              {/each}
            </select>
          </label>
        </div>
        <div class="builder-actions">
          <button class="btn btn-primary btn-sm" onclick={requestSubagentDraft} disabled={builderBusy || tiers.length === 0}>
            {builderBusy ? 'Drafting...' : 'Draft'}
          </button>
        </div>

        {#if builderResponse}
          <div class="draft-preview">
            <div class="draft-status">
              <span class="badge badge-info">{builderResponse.draft_source}</span>
              {#if builderResponse.resolved_tier}
                <span class="tier-chip" title={tierSummary(builderResponse.resolved_tier)}>
                  {builderResponse.draft.default_tier}
                </span>
              {/if}
            </div>
            {#if builderResponse.warnings?.length}
              <div class="warning-list">
                {#each builderResponse.warnings as warning}
                  <span>{warning}</span>
                {/each}
              </div>
            {/if}
            <div class="draft-grid">
              <label>
                <span>Name</span>
                <input value={builderResponse.draft.name} disabled={builderResponse.draft.action === 'update'} oninput={(event) => updateDraftField('name', (event.currentTarget as HTMLInputElement).value)} />
              </label>
              <label>
                <span>Description</span>
                <input value={builderResponse.draft.description} oninput={(event) => updateDraftField('description', (event.currentTarget as HTMLInputElement).value)} />
              </label>
              <label>
                <span>Tier</span>
                <select value={builderResponse.draft.default_tier} onchange={(event) => updateDraftField('default_tier', (event.currentTarget as HTMLSelectElement).value)}>
                  {#each tiers as tier}
                    <option value={tier.name} disabled={!!tier.error}>{tier.name}</option>
                  {/each}
                </select>
              </label>
              <label>
                <span>Risk Max</span>
                <select value={builderResponse.draft.tools_risk_max ?? ''} onchange={(event) => updateDraftField('tools_risk_max', (event.currentTarget as HTMLSelectElement).value)}>
                  <option value="">default</option>
                  <option value="low">low</option>
                </select>
              </label>
            </div>
            <label class="draft-textarea">
              <span>Prompt</span>
              <textarea value={builderResponse.draft.prompt} oninput={(event) => updateDraftField('prompt', (event.currentTarget as HTMLTextAreaElement).value)}></textarea>
            </label>
            <div class="draft-grid">
              <label>
                <span>Allow Tools</span>
                <textarea value={builderResponse.draft.tools_allow.join('\n')} oninput={(event) => updateDraftList('tools_allow', (event.currentTarget as HTMLTextAreaElement).value)}></textarea>
              </label>
              <label>
                <span>Deny Tools</span>
                <textarea value={builderResponse.draft.tools_deny.join('\n')} oninput={(event) => updateDraftList('tools_deny', (event.currentTarget as HTMLTextAreaElement).value)}></textarea>
              </label>
            </div>
            <div class="builder-actions">
              <button class="btn btn-primary btn-sm" onclick={applyDraft} disabled={builderApplying}>
                {builderApplying ? 'Applying...' : 'Approve & Save'}
              </button>
            </div>
          </div>
        {/if}
      </section>
    {/if}

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
              <div class="detail-actions">
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
                <button class="btn btn-ghost btn-sm" onclick={() => openEditBuilder(selectedSubagentValue)}>Edit with LLM</button>
                <button class="btn btn-danger btn-sm" disabled={archiveBusy} onclick={() => archiveSubagent(selectedSubagentValue)}>
                  {archiveConfirmName === selectedSubagentValue.name ? 'Confirm Archive' : 'Archive'}
                </button>
              </div>
            {:else}
              <span class="tier-chip" class:missing={selectedSubagentValue.tier_missing}>{tierLabel(selectedSubagentValue)}</span>
            {/if}
          </div>

          {#if archiveConfirmName === selectedSubagentValue.name}
            <div class="warning-list">
              <span>Archiving removes this profile from the active catalog. Recent run history stays visible in Runs. Recorded runs: {selectedSubagentValue.run_count}.</span>
            </div>
          {/if}

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

      {#if costFlowRuns.length > 0}
        <div class="cost-flow-panel">
          <AgentRuntimeCostFlow run={costFlowRuns[0]} />
        </div>
      {/if}

      <AgentRuntimeReplay events={replayEvents} runStatus={selectedRun.status} />

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

      <section class="detail-panel file-heatmap" aria-label="File Attention Heatmap">
        <div class="panel-title-row">
          <h3>File Attention</h3>
          <span>{fileAttentionOpsTotal()} ops</span>
        </div>
        {#if fileAttentionRows.length === 0}
          <div class="agentruntime-empty">No file tool calls captured yet.</div>
        {:else}
          <div class="file-attention-list">
            {#each fileAttentionRows as row}
              <article class="file-attention-row" class:read={fileAttentionAction(row) === 'read'} class:edit={fileAttentionAction(row) === 'edit'} class:both={fileAttentionAction(row) === 'both'}>
                <div class="file-attention-main">
                  <span class="file-path" title={row.path}>{row.path}</span>
                  <span class="file-count">{row.total} ops</span>
                </div>
                <div class="file-attention-meter">
                  <span class="heat-cell" style={`--heat: ${fileAttentionIntensity(row)}%`}></span>
                  <div class="sparkline" aria-label={`Access pattern for ${row.path}`}>
                    {#each normalizedSparkline(row.sparkline) as value}
                      <span style={`height: ${sparklineHeight(value, row.sparkline)}%`}></span>
                    {/each}
                  </div>
                </div>
                <div class="row-meta file-meta">
                  {#if row.reads}<span>{row.reads} read</span>{/if}
                  {#if row.edits}<span>{row.edits} edit</span>{/if}
                  {#if row.lists}<span>{row.lists} list</span>{/if}
                  {#if row.writes}<span>{row.writes} write</span>{/if}
                  {#if row.last_at}<span>{fmtTime(row.last_at)}</span>{/if}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>

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
  .header-actions { display: flex; gap: var(--space-2); flex-wrap: wrap; justify-content: flex-end; }
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
  .run-controls { display: grid; grid-template-columns: auto auto minmax(220px, 1fr) auto; gap: var(--space-3); align-items: end; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .run-view-mode { display: inline-flex; gap: var(--space-1); align-self: flex-start; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); padding: 3px; }
  .run-view-mode button { border: 0; background: transparent; color: var(--text-tertiary); border-radius: var(--radius-sm); padding: var(--space-2) var(--space-3); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .run-view-mode button.active { background: var(--surface-elevated); color: var(--text-primary); }
  .filter-group, .run-search-field { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; }
  .filter-label, .run-search-field span, .cost-summary-card > span { color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .filter-chip-row { display: flex; flex-wrap: wrap; gap: var(--space-1); }
  .filter-chip { min-height: 32px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-secondary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .filter-chip.active { border-color: var(--primary); background: var(--primary-muted); color: var(--primary-text); }
  .run-search-field input { width: 100%; min-height: 32px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-primary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); }
  .cost-summary-grid { display: grid; grid-template-columns: minmax(160px, 0.24fr) minmax(160px, 0.24fr) minmax(280px, 1fr); gap: var(--space-3); }
  .cost-summary-card { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .cost-summary-card strong { color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-lg); }
  .cost-summary-card small { color: var(--text-tertiary); font-size: var(--text-xs); }
  .plan-summary { gap: var(--space-2); }
  .plan-cost-list { display: flex; flex-direction: column; gap: var(--space-2); }
  .plan-cost-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--space-2); color: var(--text-secondary); font-size: var(--text-xs); }
  .plan-cost-row strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-mono); font-size: var(--text-xs); }
  .agentruntime-row { display: flex; flex-direction: column; gap: var(--space-2); }
  .run-open-button { width: 100%; display: flex; flex-direction: column; gap: var(--space-2); text-align: left; border: 0; background: transparent; color: inherit; padding: 0; font: inherit; cursor: pointer; }
  .run-open-button:hover .row-id { color: var(--primary-text); }
  .row-prompt { margin: 0; color: var(--text-tertiary); font-size: var(--text-sm); line-height: 1.45; overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
  .session-link { align-self: flex-start; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-secondary); padding: 4px var(--space-2); font: inherit; font-size: var(--text-xs); cursor: pointer; }
  .session-link:hover { border-color: var(--primary); color: var(--primary-text); }
  .subagents-summary { display: flex; justify-content: space-between; gap: var(--space-4); align-items: center; border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .tier-line { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-2); }
  .tier-chip { display: inline-flex; align-items: center; max-width: 220px; min-height: 24px; border: 1px solid var(--border-subtle); background: var(--primary-muted); color: var(--primary-text); border-radius: var(--radius-sm); padding: 2px var(--space-2); font-family: var(--font-mono); font-size: var(--text-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tier-chip.missing { background: var(--warning-muted); color: var(--warning); }
  .tier-warning { color: var(--warning); }
  .tier-editor { display: flex; flex-direction: column; gap: var(--space-1); min-width: min(220px, 100%); color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .tier-editor select { width: 100%; min-height: 32px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface-inset); color: var(--text-primary); padding: 0 var(--space-2); font: inherit; font-size: var(--text-xs); text-transform: none; }
  .builder-panel { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .builder-head, .builder-actions, .draft-status { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  .builder-head h3 { margin: 0; color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-lg); }
  .builder-form, .draft-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(180px, 0.28fr); gap: var(--space-3); align-items: start; }
  .builder-form label, .draft-grid label, .draft-textarea { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .builder-form select, .builder-form textarea,
  .draft-grid input, .draft-grid select, .draft-grid textarea,
  .draft-textarea textarea {
    width: 100%;
    min-width: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    padding: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-transform: none;
  }
  .builder-form textarea { min-height: 84px; resize: vertical; }
  .draft-grid textarea { min-height: 72px; resize: vertical; }
  .draft-textarea textarea { min-height: 180px; resize: vertical; line-height: 1.5; }
  .builder-form select:focus, .builder-form textarea:focus,
  .draft-grid input:focus, .draft-grid select:focus, .draft-grid textarea:focus,
  .draft-textarea textarea:focus { outline: none; border-color: var(--primary); box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.22); }
  .draft-preview { display: flex; flex-direction: column; gap: var(--space-3); border-top: 1px solid var(--border-subtle); padding-top: var(--space-3); }
  .warning-list { display: flex; flex-direction: column; gap: var(--space-1); border: 1px solid rgba(224, 145, 69, 0.3); background: rgba(224, 145, 69, 0.08); border-radius: var(--radius-sm); padding: var(--space-2); color: var(--warning); font-size: var(--text-xs); }
  .detail-actions { display: flex; align-items: flex-end; justify-content: flex-end; gap: var(--space-2); flex-wrap: wrap; }
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
  .panel-title-row { display: flex; justify-content: space-between; gap: var(--space-3); align-items: center; margin-bottom: var(--space-3); }
  .panel-title-row h3 { margin: 0; }
  .panel-title-row span { color: var(--text-ghost); font-family: var(--font-mono); font-size: var(--text-xs); }
  .file-heatmap { overflow: hidden; }
  .file-attention-list { display: flex; flex-direction: column; gap: 0; }
  .file-attention-row { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(180px, 0.42fr) minmax(160px, 0.34fr); gap: var(--space-3); align-items: center; border-top: 1px solid var(--border-subtle); padding: var(--space-3) 0; }
  .file-attention-row:first-child { border-top: 0; padding-top: 0; }
  .file-attention-row:last-child { padding-bottom: 0; }
  .file-attention-main { min-width: 0; display: grid; gap: 3px; }
  .file-path { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-family: var(--font-mono); font-size: var(--text-xs); }
  .file-count { color: var(--text-ghost); font-size: var(--text-xs); }
  .file-attention-meter { min-width: 0; display: grid; grid-template-columns: 56px minmax(72px, 1fr); gap: var(--space-2); align-items: center; }
  .heat-cell { display: block; width: 56px; height: 18px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: linear-gradient(90deg, var(--primary) var(--heat), var(--surface-inset) var(--heat)); opacity: 0.92; }
  .file-attention-row.edit .heat-cell { background: linear-gradient(90deg, var(--warning) var(--heat), var(--surface-inset) var(--heat)); }
  .file-attention-row.both .heat-cell { background: linear-gradient(90deg, var(--info) var(--heat), var(--surface-inset) var(--heat)); }
  .sparkline { display: flex; align-items: end; gap: 2px; height: 22px; min-width: 0; }
  .sparkline span { flex: 1 1 4px; min-width: 3px; max-width: 12px; border-radius: 2px 2px 0 0; background: var(--text-tertiary); opacity: 0.75; }
  .file-attention-row.read .sparkline span { background: var(--primary); }
  .file-attention-row.edit .sparkline span { background: var(--warning); }
  .file-attention-row.both .sparkline span { background: var(--info); }
  .file-meta { justify-content: flex-end; gap: var(--space-2); min-width: 0; }
  .file-meta span { white-space: nowrap; }
  pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); background: var(--surface-elevated); padding: var(--space-3); border-radius: var(--radius-md); }
  .event-log { display: flex; flex-direction: column; gap: var(--space-2); }
  .event-row { display: flex; gap: var(--space-3); align-items: flex-start; border-top: 1px solid var(--border-subtle); padding-top: var(--space-2); }
  .event-type { min-width: 140px; color: var(--primary); font-size: var(--text-xs); }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 960px) { .intro-card, .run-controls, .cost-summary-grid { grid-template-columns: 1fr; } }
  @media (max-width: 960px) { .subagents-layout { grid-template-columns: 1fr; } }
  @media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
  @media (max-width: 768px) { .variants-grid, .prompt-grid, .policy-grid, .recent-run, .builder-form, .draft-grid, .plan-cost-row, .file-attention-row { grid-template-columns: 1fr; } .file-meta { justify-content: flex-start; } .subagents-summary, .detail-head, .agentruntime-header { align-items: stretch; flex-direction: column; } .detail-actions, .header-actions { justify-content: flex-start; } }
</style>
