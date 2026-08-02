<script lang="ts">
  import { onDestroy } from 'svelte'
  import { t } from '../i18n'
  import AgentRuntimeCostFlow from './AgentRuntimeCostFlow.svelte'
  import AgentRuntimeFlowGraph from './AgentRuntimeFlowGraph.svelte'
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
    recommendAgentRuntimeSubagents,
    restartAgentRuntimeRun,
    streamAgentRuntimeRunEvents,
    updateAgentRuntimeSubagentTier,
  } from '../lib/api'
  import type {
    AgentRuntimeSubagent,
    AgentRuntimeSubagentDraft,
    AgentRuntimeSubagentDraftResponse,
    AgentRuntimeSubagentRecommendation,
    AgentRuntimeSubagentRecommendationsResponse,
    AgentRuntimeSubagentsResponse,
    AgentRuntimeTierOption,
    ConsensusVariantRecord,
    FileAttentionSummary,
    AgentRuntimeDiffFileChange,
    AgentRuntimeDiffTimelineEntry,
    AgentRuntimeRun,
    AgentRuntimeRunCheckpoint,
    AgentRuntimeRunEvent,
    AgentRuntimeRecoveryMode,
  } from '../lib/types'

  interface Props {
    runId?: string
    tab?: 'runs' | 'subagents'
    onNavigate: (path: string) => void
  }

  type RunStatusFilter = 'all' | 'running' | 'done' | 'failed'
  type RunTimeRange = '24h' | '7d' | 'all'
  type RunViewMode = 'list' | 'tree' | 'gantt' | 'flow'
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
  let recommendationBusy = $state(false)
  let recommendationResponse: AgentRuntimeSubagentRecommendationsResponse | null = $state(null)
  let archiveConfirmName = $state('')
  let archiveBusy = $state(false)
  let restartCheckpointID = $state('')
  let restartAgent = $state('')
  let restartTier = $state('')
  let restartAlias = $state('')
  let restartModel = $state('')
  let restartPromptAdjustment = $state('')
  let restartMode: AgentRuntimeRecoveryMode = $state('retry_from_prompt')
  let confirmUnsafeRecovery = $state(false)
  let restartBusy = $state(false)
  let restartMessage = $state('')
  let stopStream: (() => void) | null = null

  let runtimeTools = $derived([
    { name: 'subagents_run', detail: $t.agentRuntime.toolDetail.run },
    { name: 'subagents_orchestrate', detail: $t.agentRuntime.toolDetail.orchestrate },
    { name: 'subagents_plan', detail: $t.agentRuntime.toolDetail.plan },
  ])

  const starterPrompts = [
    'Analyze this codebase in parallel with three subagents.',
    'Ask two subagents to inspect the frontend and backend separately.',
  ]

  let runStatusOptions = $derived<{ value: RunStatusFilter; label: string }[]>([
    { value: 'all', label: $t.agentRuntime.statusAll },
    { value: 'running', label: $t.agentRuntime.statusRunning },
    { value: 'done', label: $t.agentRuntime.statusDone },
    { value: 'failed', label: $t.agentRuntime.statusFailed },
  ])

  let runTimeRangeOptions = $derived<{ value: RunTimeRange; label: string }[]>([
    { value: '24h', label: $t.agentRuntime.range24h },
    { value: '7d', label: $t.agentRuntime.range7d },
    { value: 'all', label: $t.agentRuntime.rangeAll },
  ])

  let runViewModeOptions = $derived<{ value: RunViewMode; label: string }[]>([
    { value: 'list', label: $t.agentRuntime.viewMode.list },
    { value: 'tree', label: $t.agentRuntime.viewMode.tree },
    { value: 'gantt', label: $t.agentRuntime.viewMode.gantt },
    { value: 'flow', label: $t.agentRuntime.viewMode.flow },
  ])

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
  let diffTimelineEntries = $derived.by<AgentRuntimeDiffTimelineEntry[]>(() => {
    return [...(selectedRun?.diff_timeline ?? [])]
      .sort((a, b) => runTimelineTimestamp(a) - runTimelineTimestamp(b))
  })
  let costFlowRuns = $derived.by<AgentRuntimeRun[]>(() => selectedRun ? [selectedRun] : [])
  let replayEvents = $derived.by<AgentRuntimeRunEvent[]>(() => events)
  let selectedRunCheckpoints = $derived.by<AgentRuntimeRunCheckpoint[]>(() => selectedRun?.checkpoints ?? [])
  let selectedRestartCheckpoint = $derived.by<AgentRuntimeRunCheckpoint | null>(() => {
    return selectedRunCheckpoints.find((checkpoint) => checkpoint.checkpoint_id === restartCheckpointID)
      ?? selectedRunCheckpoints[selectedRunCheckpoints.length - 1]
      ?? null
  })

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
      error = e instanceof Error ? e.message : $t.agentRuntime.failedLoadRuns
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
      error = e instanceof Error ? e.message : $t.agentRuntime.failedLoadSubagents
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
      prepareRestartForm(selectedRun)
    } catch (e) {
      selectedRun = null
      error = e instanceof Error ? e.message : $t.agentRuntime.failedLoadRun
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

  function runTimelineTimestamp(entry: AgentRuntimeDiffTimelineEntry): number {
    for (const value of [entry.started_at, entry.completed_at]) {
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

  function diffTimelineSummary(entry: AgentRuntimeDiffTimelineEntry): string {
    const files = entry.summary?.files ?? entry.files?.length ?? 0
    const additions = entry.summary?.additions ?? 0
    const deletions = entry.summary?.deletions ?? 0
    return `${files} ${files === 1 ? 'file' : 'files'} · +${additions} -${deletions}`
  }

  function diffFileStats(file: AgentRuntimeDiffFileChange): string {
    return `+${file.additions ?? 0} -${file.deletions ?? 0}`
  }

  function diffFileInspectorTitle(file: AgentRuntimeDiffFileChange): string {
    const target = file.git_inspector_url?.trim()
    return target ? `Git Inspector target: ${target}` : 'Git Inspector target unavailable'
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

  function isRestartable(run: AgentRuntimeRun | null): boolean {
    return (run?.status === 'failed' || run?.status === 'canceled') && (run.checkpoints?.length ?? 0) > 0
  }

  function prepareRestartForm(run: AgentRuntimeRun | null) {
    restartMessage = ''
    if (!isRestartable(run)) {
      restartCheckpointID = ''
      restartAgent = ''
      restartTier = ''
      restartAlias = ''
      restartModel = ''
      restartPromptAdjustment = ''
      restartMode = 'retry_from_prompt'
      confirmUnsafeRecovery = false
      return
    }
    const checkpoints = run?.checkpoints ?? []
    const latest = checkpoints[checkpoints.length - 1]
    restartCheckpointID = latest?.checkpoint_id ?? ''
    restartAgent = run?.agent ?? ''
    restartTier = run?.tier ?? ''
    restartAlias = run?.provider_override?.alias ?? ''
    restartModel = run?.provider_override?.model ?? ''
    restartPromptAdjustment = ''
    restartMode = preferredRecoveryMode(latest)
    confirmUnsafeRecovery = false
  }

  function checkpointLabel(checkpoint: AgentRuntimeRunCheckpoint): string {
    return [checkpoint.label || checkpoint.kind || checkpoint.checkpoint_id, checkpoint.capability || 'retry_only', fmtTime(checkpoint.created_at)]
      .filter((item) => item && item !== '—')
      .join(' · ')
  }

  function selectedCheckpoint(): AgentRuntimeRunCheckpoint | null {
    return selectedRunCheckpoints.find((checkpoint) => checkpoint.checkpoint_id === restartCheckpointID) ?? selectedRunCheckpoints.at(-1) ?? null
  }

  function recoveryModes(checkpoint: AgentRuntimeRunCheckpoint | null): AgentRuntimeRecoveryMode[] {
    return checkpoint?.recovery_modes?.length ? checkpoint.recovery_modes : ['retry_from_prompt']
  }

  function preferredRecoveryMode(checkpoint: AgentRuntimeRunCheckpoint | null): AgentRuntimeRecoveryMode {
    const modes = recoveryModes(checkpoint)
    if (modes.includes('replay_from_checkpoint')) return 'replay_from_checkpoint'
    if (modes.includes('resume_from_checkpoint')) return 'resume_from_checkpoint'
    return 'retry_from_prompt'
  }

  function recoveryModeLabel(mode: AgentRuntimeRecoveryMode): string {
    if (mode === 'replay_from_checkpoint') return 'Replay from checkpoint'
    if (mode === 'resume_from_checkpoint') return 'Resume from checkpoint'
    return 'Retry from prompt'
  }

  function handleRestartCheckpointChange() {
    restartMode = preferredRecoveryMode(selectedCheckpoint())
    confirmUnsafeRecovery = false
  }

  function recoveryActionLabel(): string {
    if (restartBusy) return 'Starting...'
    return recoveryModeLabel(restartMode)
  }

  async function restartSelectedRun() {
    if (!selectedRun || restartBusy) return
    restartBusy = true
    restartMessage = ''
    error = ''
    try {
      const alias = restartAlias.trim()
      const model = restartModel.trim()
      const checkpoint = selectedCheckpoint()
      const availableModes = recoveryModes(checkpoint)
      const mode = availableModes.includes(restartMode) ? restartMode : preferredRecoveryMode(checkpoint)
      const retry = await restartAgentRuntimeRun(selectedRun.run_id, {
        checkpoint_id: restartCheckpointID || undefined,
        agent: restartAgent.trim() || undefined,
        tier: restartTier.trim() || undefined,
        provider_override: alias || model ? { alias, model } : undefined,
        prompt_adjustment: restartPromptAdjustment.trim() || undefined,
        mode,
        confirm_unsafe_recovery: checkpoint?.recovery_approval_required ? confirmUnsafeRecovery : undefined,
      })
      restartMessage = `Started ${retry.run_id}`
      openRunDetail(retry.run_id)
    } catch (e) {
      error = e instanceof Error ? e.message : $t.agentRuntime.failedRestartRun
    } finally {
      restartBusy = false
    }
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
      error = e instanceof Error ? e.message : $t.agentRuntime.failedUpdateTier
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

  async function requestSubagentRecommendations() {
    recommendationBusy = true
    error = ''
    try {
      recommendationResponse = await recommendAgentRuntimeSubagents({ limit: 120, min_runs: 2 })
    } catch (e) {
      error = e instanceof Error ? e.message : $t.agentRuntime.failedRecommend
    } finally {
      recommendationBusy = false
    }
  }

  function reviewRecommendation(recommendation: AgentRuntimeSubagentRecommendation) {
    builderOpen = true
    builderMode = recommendation.draft.action === 'update' ? 'edit' : 'create'
    builderBaseName = recommendation.draft.action === 'update' ? recommendation.draft.name : ''
    builderRequest = recommendation.reason
    builderTier = recommendation.draft.default_tier || defaultBuilderTier()
    builderResponse = {
      draft: recommendation.draft,
      draft_source: 'recommendation',
      warnings: [],
      tiers: recommendationResponse?.tiers?.length ? recommendationResponse.tiers : tiers,
      resolved_tier: recommendation.resolved_tier,
    }
    archiveConfirmName = ''
  }

  function confidenceLabel(value: number): string {
    if (!Number.isFinite(value)) return '—'
    return `${Math.round(value * 100)}%`
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
      error = e instanceof Error ? e.message : $t.agentRuntime.failedDraft
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
    const appliedName = builderResponse.draft.name
    const appliedSource = builderResponse.draft_source
    builderApplying = true
    error = ''
    try {
      const updated = await applyAgentRuntimeSubagentDraft(builderResponse.draft)
      await loadSubagents()
      selectedSubagentName = updated.name
      if (appliedSource === 'recommendation' && recommendationResponse) {
        const remaining = recommendationResponse.recommendations.filter((item) => item.draft.name !== appliedName)
        recommendationResponse = {
          ...recommendationResponse,
          count: remaining.length,
          recommendations: remaining,
        }
      }
      closeBuilder()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.agentRuntime.failedApplyDraft
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
      error = e instanceof Error ? e.message : $t.agentRuntime.failedArchive
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
      <div class="agentruntime-title">{$t.agentRuntime.title}</div>
      <div class="agentruntime-subtitle">
        {#if runId}
          {$t.agentRuntime.subtitleRunDetail}
        {:else if activeTab === 'subagents'}
          {$t.agentRuntime.subtitleSubagents}
        {:else}
          {$t.agentRuntime.subtitleRuns}
        {/if}
      </div>
    </div>
    {#if runId}
      <button class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/agentruntime')}>{$t.agentRuntime.back}</button>
    {:else if activeTab === 'subagents'}
      <div class="header-actions">
        <button class="btn btn-secondary btn-sm" onclick={requestSubagentRecommendations} disabled={recommendationBusy}>
          {recommendationBusy ? $t.agentRuntime.analyzing : $t.agentRuntime.recommendFromRuns}
        </button>
        <button class="btn btn-primary btn-sm" onclick={openCreateBuilder}>{$t.agentRuntime.newSubagent}</button>
        <button class="btn btn-ghost btn-sm" onclick={loadSubagents} disabled={loading}>{loading ? $t.agentRuntime.loading : $t.agentRuntime.refresh}</button>
      </div>
    {:else}
      <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? $t.agentRuntime.loading : $t.agentRuntime.refresh}</button>
    {/if}
  </div>

  {#if !runId}
    <div class="runtime-tabs" role="tablist" aria-label={$t.agentRuntime.tabsAriaLabel}>
      <button
        type="button"
        class:active={activeTab === 'runs'}
        onclick={() => onNavigate('/console/agentruntime')}
        role="tab"
        aria-selected={activeTab === 'runs'}
      >
        {$t.agentRuntime.tabRuns}
      </button>
      <button
        type="button"
        class:active={activeTab === 'subagents'}
        onclick={() => onNavigate('/console/agentruntime/subagents')}
        role="tab"
        aria-selected={activeTab === 'subagents'}
      >
        {$t.agentRuntime.tabSubagents}
      </button>
    </div>
  {/if}

  {#if !runId && activeTab === 'runs'}
    <section class="intro-card" aria-labelledby="agentruntime-intro-title">
      <div class="intro-copy">
        <div class="eyebrow">{$t.agentRuntime.introEyebrow}</div>
        <h2 id="agentruntime-intro-title">{$t.agentRuntime.introTitle}</h2>
        <p>{$t.agentRuntime.introBody}</p>
      </div>
      <div class="tool-strip" aria-label={$t.agentRuntime.toolStripAriaLabel}>
        {#each runtimeTools as tool}
          <div class="tool-chip">
            <code>{tool.name}</code>
            <span>{tool.detail}</span>
          </div>
        {/each}
      </div>
    </section>

    <section class="run-controls" aria-label={$t.agentRuntime.filtersAriaLabel}>
      <div class="filter-group">
        <span class="filter-label">{$t.agentRuntime.filterStatus}</span>
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
        <span class="filter-label">{$t.agentRuntime.filterTimeRange}</span>
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
        <span>{$t.agentRuntime.filterSearchLabel}</span>
        <input
          bind:value={runSearchInput}
          onkeydown={handleRunSearchKeydown}
          placeholder={$t.agentRuntime.filterSearchPlaceholder}
        />
      </label>
      <button class="btn btn-primary btn-sm" type="button" onclick={loadRuns} disabled={loading}>
        {loading ? $t.agentRuntime.loading : $t.agentRuntime.apply}
      </button>
    </section>

    <section class="run-view-mode" aria-label={$t.agentRuntime.visualAriaLabel}>
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

    <section class="cost-summary-grid" aria-label={$t.agentRuntime.costSummaryAriaLabel}>
      <div class="cost-summary-card">
        <span>{$t.agentRuntime.costToday}</span>
        <strong>{fmtUSD(todayCostUSD)}</strong>
        <small>{$t.agentRuntime.costLoadedRunCosts}</small>
      </div>
      <div class="cost-summary-card">
        <span>{$t.agentRuntime.cost7d}</span>
        <strong>{fmtUSD(sevenDayCostUSD)}</strong>
        <small>{$t.agentRuntime.costLoadedRunCosts}</small>
      </div>
      <div class="cost-summary-card plan-summary">
        <span>{$t.agentRuntime.costPlanTotals}</span>
        {#if planCostRows.length === 0}
          <small>{$t.agentRuntime.costNoData}</small>
        {:else}
          <div class="plan-cost-list">
            {#each planCostRows as row}
              <div class="plan-cost-row">
                <strong title={row.key}>{row.label}</strong>
                <span>{fmtUSD(row.total)} · {$t.agentRuntime.costRunSuffix(row.runs)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    {#if error}
      <div class="error-banner">{error}</div>
    {/if}
    {#if runViewMode === 'flow' && runs.length > 0}
      <AgentRuntimeFlowGraph {runs} onSelectRun={openRunDetail} />
    {:else if runViewMode === 'tree' && runs.length > 0}
      <AgentRuntimeTree {runs} onSelectRun={openRunDetail} />
    {:else if runViewMode === 'gantt' && runs.length > 0}
      <AgentRuntimeGantt {runs} onSelectRun={openRunDetail} />
    {:else}
    <div class="agentruntime-list">
      {#if runs.length === 0 && !loading}
        {#if runFiltersActive()}
          <section class="empty-guide" aria-labelledby="agentruntime-empty-title">
            <div>
              <div class="eyebrow">{$t.agentRuntime.emptyNoMatchEyebrow}</div>
              <h3 id="agentruntime-empty-title">{$t.agentRuntime.emptyNoMatchTitle}</h3>
              <p>{$t.agentRuntime.emptyNoMatchBody}</p>
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
                {$t.agentRuntime.clearFilters}
              </button>
              <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? $t.agentRuntime.loading : $t.agentRuntime.refresh}</button>
            </div>
          </section>
        {:else}
          <section class="empty-guide" aria-labelledby="agentruntime-empty-title">
            <div>
              <div class="eyebrow">{$t.agentRuntime.emptyNoRunsEyebrow}</div>
              <h3 id="agentruntime-empty-title">{$t.agentRuntime.emptyNoRunsTitle}</h3>
              <p>{$t.agentRuntime.emptyNoRunsBody}</p>
            </div>
            <div class="prompt-grid">
              {#each starterPrompts as prompt}
                <blockquote>{prompt}</blockquote>
              {/each}
            </div>
            <div class="empty-actions">
              <button class="btn btn-secondary btn-sm" onclick={() => onNavigate('/console/chat')}>{$t.agentRuntime.openChat}</button>
              <button class="btn btn-ghost btn-sm" onclick={loadRuns} disabled={loading}>{loading ? $t.agentRuntime.loading : $t.agentRuntime.refresh}</button>
            </div>
          </section>
        {/if}
      {:else}
        {#each runs as run}
          <article class="agentruntime-row">
            <button class="run-open-button" type="button" onclick={() => openRunDetail(run.run_id)}>
              <div class="row-main">
                <span class="row-id">{run.run_id}</span>
                <span class="row-agent">{run.agent || $t.agentRuntime.rowAgentDefault}</span>
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
                {$t.agentRuntime.sessionLink(shortID(run.session_id))}
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

    {#if recommendationResponse}
      <section class="recommendation-panel" aria-label="Recommended subagent profiles">
        <div class="recommendation-head">
          <div>
            <div class="eyebrow">Run Patterns</div>
            <h3>Recommended Profiles</h3>
          </div>
          <span>{recommendationResponse.count} of {recommendationResponse.analyzed_run_count} analyzed</span>
        </div>
        {#if recommendationResponse.warnings?.length}
          <div class="warning-list">
            {#each recommendationResponse.warnings as warning}
              <span>{warning}</span>
            {/each}
          </div>
        {/if}
        {#if recommendationResponse.recommendations.length === 0}
          <div class="agentruntime-empty">No repeated run pattern is ready to save as a profile.</div>
        {:else}
          <div class="recommendation-grid">
            {#each recommendationResponse.recommendations as recommendation}
              <article class="recommendation-card">
                <div class="recommendation-card-head">
                  <div>
                    <strong>{recommendation.draft.name}</strong>
                    <p>{recommendation.title}</p>
                  </div>
                  <span class="tier-chip">{confidenceLabel(recommendation.confidence)}</span>
                </div>
                <p>{recommendation.reason}</p>
                <div class="recommendation-meta">
                  <span>{recommendation.run_count} runs</span>
                  {#if recommendation.draft.default_tier}<span>{recommendation.draft.default_tier}</span>{/if}
                  {#if recommendation.keywords.length}<span>{recommendation.keywords.slice(0, 4).join(', ')}</span>{/if}
                </div>
                <div class="recommendation-meta">
                  <span>{recommendation.draft.tools_allow.join(', ')}</span>
                </div>
                <div class="recommendation-run-list">
                  {#each recommendation.recent_run_ids as runID}
                    <button type="button" class="run-inline-link" onclick={() => openRunDetail(runID)}>
                      Run {shortID(runID)}
                    </button>
                  {/each}
                </div>
                <div class="builder-actions">
                  <button class="btn btn-primary btn-sm" onclick={() => reviewRecommendation(recommendation)}>
                    Review Draft
                  </button>
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

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
          {#if selectedRun.restarted_from_run_id}
            <div><span class="label">Restarted From</span><button class="run-inline-link" type="button" onclick={() => openRunDetail(selectedRun?.restarted_from_run_id ?? '')}>{shortID(selectedRun.restarted_from_run_id)}</button></div>
          {/if}
          {#if selectedRun.restart_attempt}
            <div><span class="label">Attempt</span><span>#{selectedRun.restart_attempt}</span></div>
          {/if}
          {#if selectedRun.recovery_mode}
            <div><span class="label">Recovery Mode</span><span>{recoveryModeLabel(selectedRun.recovery_mode)}</span></div>
          {/if}
        </div>
      </div>

      {#if isRestartable(selectedRun)}
        <section class="detail-panel restart-panel" aria-label="Recover Run">
          <div class="panel-title-row">
            <h3>Recover</h3>
            <span>{selectedRunCheckpoints.length} checkpoints</span>
          </div>
          <div class="restart-grid">
            <label>
              <span>Checkpoint</span>
              <select bind:value={restartCheckpointID} onchange={handleRestartCheckpointChange}>
                {#each selectedRunCheckpoints as checkpoint}
                  <option value={checkpoint.checkpoint_id}>{checkpointLabel(checkpoint)}</option>
                {/each}
              </select>
            </label>
            <label>
              <span>Recovery Mode</span>
              <select bind:value={restartMode}>
                {#each recoveryModes(selectedCheckpoint()) as mode}
                  <option value={mode}>{recoveryModeLabel(mode)}</option>
                {/each}
              </select>
            </label>
            <label>
              <span>Agent</span>
              <input bind:value={restartAgent} placeholder="default" />
            </label>
            <label>
              <span>Tier</span>
              <input bind:value={restartTier} placeholder="standard" />
            </label>
            <label>
              <span>Provider Alias</span>
              <input bind:value={restartAlias} placeholder="alias" />
            </label>
            <label>
              <span>Model</span>
              <input bind:value={restartModel} placeholder="model" />
            </label>
            <label class="restart-adjustment">
              <span>Prompt Adjustment</span>
              <textarea bind:value={restartPromptAdjustment} rows="3" placeholder="Retry with a narrower assumption or alternate permission set."></textarea>
            </label>
          </div>
          {#if selectedRestartCheckpoint}
            <div class="checkpoint-safety" class:checkpoint-warning={selectedRestartCheckpoint.recovery_approval_required}>
              <div>
                <strong>{selectedRestartCheckpoint.format || 'prompt_checkpoint_v0'} · {selectedRestartCheckpoint.capability || 'retry_only'}</strong>
                <span>{selectedRestartCheckpoint.resumable ? 'Resumable' : 'Not resumable'} · {selectedRestartCheckpoint.resume_reason || 'No resume metadata recorded.'}</span>
              </div>
              <div class="row-meta">
                <span>{selectedRestartCheckpoint.tool_result_refs?.length ?? 0} tool results</span>
                <span>{selectedRestartCheckpoint.effect_receipt_refs?.length ?? 0} effect receipts</span>
                <span>next: {selectedRestartCheckpoint.next_action || 'retry_prompt'}</span>
              </div>
              {#if selectedRestartCheckpoint.recovery_approval_required}
                <label class="unsafe-recovery-confirm">
                  <input type="checkbox" bind:checked={confirmUnsafeRecovery} />
                  <span>Human decision: continue despite an effect without a committed receipt. {selectedRestartCheckpoint.recovery_approval_reason || ''}</span>
                </label>
              {/if}
            </div>
          {/if}
          <div class="detail-actions">
            <button class="btn btn-primary btn-sm" type="button" disabled={restartBusy || Boolean(selectedCheckpoint()?.recovery_approval_required && !confirmUnsafeRecovery)} onclick={restartSelectedRun}>
              {recoveryActionLabel()}
            </button>
            {#if restartMessage}<span class="row-meta">{restartMessage}</span>{/if}
          </div>
        </section>
      {/if}

      {#if costFlowRuns.length > 0}
        <div class="cost-flow-panel">
          <AgentRuntimeCostFlow run={costFlowRuns[0]} />
        </div>
      {/if}

      <AgentRuntimeReplay events={replayEvents} runStatus={selectedRun.status} />

      <section class="detail-panel diff-timeline" aria-label="Diff Timeline">
        <div class="panel-title-row">
          <h3>Diff Timeline</h3>
          <span>{diffTimelineEntries.length} entries</span>
        </div>
        {#if diffTimelineEntries.length === 0}
          <div class="agentruntime-empty">No git diff snapshots captured for this run.</div>
        {:else}
          <div class="diff-timeline-list">
            {#each diffTimelineEntries as entry}
              <article class="diff-entry">
                <div class="diff-entry-head">
                  <div>
                    <button class="run-inline-link" type="button" onclick={() => openRunDetail(entry.run_id)}>
                      Run {shortID(entry.run_id)}
                    </button>
                    <div class="row-meta">
                      {#if entry.agent}<span>{entry.agent}</span>{/if}
                      {#if entry.session_id}<span>session {shortID(entry.session_id)}</span>{/if}
                      {#if entry.step_id}<span>step {entry.step_id}</span>{/if}
                      {#if entry.completed_at}<span>{fmtTime(entry.completed_at)}</span>{/if}
                    </div>
                  </div>
                  <div class="diff-summary">
                    <span>{diffTimelineSummary(entry)}</span>
                    {#if entry.repo_root}<small title={entry.repo_root}>{entry.repo_root}</small>{/if}
                  </div>
                </div>
                {#if entry.prompt}
                  <p class="row-prompt">{entry.prompt}</p>
                {/if}
                <div class="diff-file-list">
                  {#each entry.files ?? [] as file}
                    <article class="diff-file-row">
                      <div class="diff-file-head">
                        <span class="file-path" title={file.path}>{file.path}</span>
                        <span class="diff-status">{file.status}</span>
                        <span class="diff-stats">{diffFileStats(file)}</span>
                        <button
                          class="git-inspector-chip"
                          type="button"
                          disabled
                          title={diffFileInspectorTitle(file)}
                        >
                          Git Inspector
                        </button>
                      </div>
                      <details class="diff-preview" open={entry.files?.length === 1}>
                        <summary>Diff preview</summary>
                        <pre>{file.patch || 'No patch preview available for this change.'}</pre>
                      </details>
                    </article>
                  {/each}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>

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
  .recommendation-panel { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .recommendation-head, .recommendation-card-head { display: flex; justify-content: space-between; gap: var(--space-3); align-items: flex-start; }
  .recommendation-head h3 { margin: 0; color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-lg); }
  .recommendation-head > span { color: var(--text-ghost); font-family: var(--font-mono); font-size: var(--text-xs); }
  .recommendation-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: var(--space-3); }
  .recommendation-card { display: flex; flex-direction: column; gap: var(--space-3); min-width: 0; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); padding: var(--space-3); }
  .recommendation-card strong { display: block; color: var(--text-primary); font-family: var(--font-mono); font-size: var(--text-sm); }
  .recommendation-card p { margin: 0; color: var(--text-secondary); font-size: var(--text-sm); line-height: 1.45; }
  .recommendation-meta, .recommendation-run-list { display: flex; gap: var(--space-2); align-items: center; flex-wrap: wrap; color: var(--text-tertiary); font-size: var(--text-xs); }
  .recommendation-meta span { min-width: 0; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .builder-panel { display: flex; flex-direction: column; gap: var(--space-3); border: 1px solid var(--border-subtle); background: var(--surface); border-radius: var(--radius-md); padding: var(--space-3); }
  .builder-head, .builder-actions, .draft-status { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  .builder-head h3 { margin: 0; color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-lg); }
  .builder-form, .draft-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(180px, 0.28fr); gap: var(--space-3); align-items: start; }
  .builder-form label, .draft-grid label, .draft-textarea, .restart-grid label { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .builder-form select, .builder-form textarea,
  .draft-grid input, .draft-grid select, .draft-grid textarea,
  .draft-textarea textarea,
  .restart-grid input, .restart-grid select, .restart-grid textarea {
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
  .restart-grid textarea { min-height: 72px; resize: vertical; }
  .draft-textarea textarea { min-height: 180px; resize: vertical; line-height: 1.5; }
  .builder-form select:focus, .builder-form textarea:focus,
  .draft-grid input:focus, .draft-grid select:focus, .draft-grid textarea:focus,
  .draft-textarea textarea:focus,
  .restart-grid input:focus, .restart-grid select:focus, .restart-grid textarea:focus { outline: none; border-color: var(--primary); box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.22); }
  .draft-preview { display: flex; flex-direction: column; gap: var(--space-3); border-top: 1px solid var(--border-subtle); padding-top: var(--space-3); }
  .warning-list { display: flex; flex-direction: column; gap: var(--space-1); border: 1px solid rgba(224, 145, 69, 0.3); background: rgba(224, 145, 69, 0.08); border-radius: var(--radius-sm); padding: var(--space-2); color: var(--warning); font-size: var(--text-xs); }
  .detail-actions { display: flex; align-items: flex-end; justify-content: flex-end; gap: var(--space-2); flex-wrap: wrap; }
  .restart-panel { display: flex; flex-direction: column; gap: var(--space-3); }
  .restart-grid { display: grid; grid-template-columns: minmax(180px, 1.2fr) repeat(5, minmax(120px, 0.7fr)); gap: var(--space-3); align-items: start; }
  .restart-adjustment { grid-column: 1 / -1; }
  .checkpoint-safety { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-3); border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-raised); }
  .checkpoint-safety > div:first-child { display: flex; flex-direction: column; gap: var(--space-1); }
  .checkpoint-safety strong { font-family: var(--font-mono); font-size: 11px; color: var(--text); }
  .checkpoint-safety span { color: var(--text-muted); }
  .checkpoint-warning { border-color: var(--warning); background: color-mix(in srgb, var(--warning) 9%, var(--surface-raised)); }
  .unsafe-recovery-confirm { display: flex; flex-direction: row; align-items: flex-start; gap: var(--space-2); color: var(--warning); font-family: var(--font-sans); font-size: 12px; text-transform: none; }
  .unsafe-recovery-confirm input { margin-top: 2px; }
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
  .diff-timeline-list, .diff-file-list { display: flex; flex-direction: column; gap: var(--space-3); }
  .diff-entry { display: flex; flex-direction: column; gap: var(--space-3); border-top: 1px solid var(--border-subtle); padding-top: var(--space-3); }
  .diff-entry:first-child { border-top: 0; padding-top: 0; }
  .diff-entry-head, .diff-file-head { display: flex; justify-content: space-between; gap: var(--space-3); align-items: flex-start; flex-wrap: wrap; }
  .run-inline-link { border: 0; background: transparent; color: var(--primary-text); padding: 0; font: inherit; font-family: var(--font-mono); font-size: var(--text-xs); cursor: pointer; }
  .run-inline-link:hover { text-decoration: underline; }
  .diff-summary { display: grid; justify-items: end; gap: 2px; color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--text-xs); }
  .diff-summary small { max-width: min(480px, 60vw); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-ghost); }
  .diff-file-row { display: flex; flex-direction: column; gap: var(--space-2); border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); padding: var(--space-3); }
  .diff-status, .diff-stats, .git-inspector-chip { border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); background: var(--surface); color: var(--text-secondary); padding: 2px var(--space-2); font-family: var(--font-mono); font-size: var(--text-xs); }
  .diff-stats { color: var(--primary-text); }
  .git-inspector-chip:disabled { opacity: 0.66; cursor: default; }
  .diff-preview summary { color: var(--text-secondary); cursor: pointer; font-size: var(--text-xs); margin-bottom: var(--space-2); }
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
  @media (max-width: 768px) { .variants-grid, .prompt-grid, .policy-grid, .recent-run, .builder-form, .draft-grid, .restart-grid, .plan-cost-row, .file-attention-row { grid-template-columns: 1fr; } .file-meta { justify-content: flex-start; } .subagents-summary, .detail-head, .agentruntime-header, .diff-entry-head, .recommendation-head, .recommendation-card-head { align-items: stretch; flex-direction: column; } .detail-actions, .header-actions, .diff-summary { justify-content: flex-start; justify-items: start; } }
</style>
