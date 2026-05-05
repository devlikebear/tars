import type {
  APIErrorPayload,
  Approval,
  AutomationAuditListResponse,
  ChatEvent,
  ChatRequest,
  ChatTierRecommendationRequest,
  CleanupApplyResult,
  CleanupPlan,
  ConfigFile,
  ConfigSchema,
  HealthzResponse,
  ProvidersAPIInfo,
  ProviderModelsInfo,
  SetupStatusResponse,
  HubInstallResponse,
  HubInstalled,
  AgentRuntimeRun,
  AgentRuntimeRunEvent,
  AgentRuntimeProviderOverride,
  AgentRuntimeSubagent,
  AgentRuntimeSubagentArchiveResponse,
  AgentRuntimeSubagentDraft,
  AgentRuntimeSubagentDraftResponse,
  AgentRuntimeSubagentRecommendationsResponse,
  AgentRuntimeSubagentsResponse,
  HubRegistry,
  MCPServerStatus,
  MemoryAsset,
  MemoryCandidateAction,
  MemoryCandidateListResponse,
  MemoryCandidateReviewResponse,
  MemoryCandidateStatus,
  MemoryFile,
  MemoryPrefetchResult,
  MemorySearchResult,
  SyspromptFile,
  SyspromptPreview,
  PluginDef,
  SkillDef,
  CreateCronJobRequest,
  CronJob,
  CronRunRecord,
  CronRunResult,
  EventsHistoryInfo,
  ForkPromotionListResponse,
  ForkPromotionResult,
  NotificationMessage,
  OpsStatus,
  PulseSnapshot,
  PulseTickOutcome,
  PulseConfigView,
  ReflectionSnapshot,
  ReflectionRunSummary,
  ReflectionConfigView,
  Session,
  SessionAutomationConsent,
  SessionStyleControl,
  SessionStyleResponse,
  SessionMessage,
  GlobalPlansResponse,
  PlanArchiveResponse,
  SkillCreatorDraftRequest,
  SkillCreatorDraftResponse,
  SkillCreatorSaveResponse,
  SkillCreatorTestResponse,
  SkillCreatorSubmitResponse,
  SkillExtractionCandidateAction,
  SkillExtractionCandidateStatus,
  SkillExtractionListResponse,
  SkillExtractionReviewResponse,
  MCPServerCreatorDraftRequest,
  MCPServerCreatorDraftResponse,
  MCPServerCreatorSaveResponse,
  MCPServerCreatorTestResponse,
  MCPServerCreatorSubmitResponse,
  UpdateCronJobRequest,
  SessionTasks,
  TaskContract,
  TaskEvidence,
  GitBranchesResponse,
  GitCommitDetail,
  GitDiff,
  GitLogResponse,
  GitMutationPlan,
  GitStatus,
  GitWorktreesResponse,
  SessionWorkDirs,
  SessionCwd,
  SessionEffectiveConfig,
  UsageToday,
  LogsResponse,
  AnalyticsResponse,
  TelegramPairingsResponse,
  TelegramAllowedUser,
} from './types'

export class APIRequestError extends Error {
  payload?: APIErrorPayload

  constructor(message: string, payload?: APIErrorPayload) {
    super(message)
    this.name = 'APIRequestError'
    this.payload = payload
  }

  get sandboxReport() {
    return this.payload?.sandbox_report
  }
}

async function requestJSON<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    credentials: 'same-origin',
    headers: {
      Accept: 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim()
    let payload: APIErrorPayload | undefined
    try {
      payload = (await response.json()) as APIErrorPayload
      if (payload?.error?.trim()) {
        message = payload.error.trim()
      }
    } catch {
      // ignore non-JSON error bodies
    }
    throw new APIRequestError(message, payload)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

function normalizeSessionTasks(data: Partial<SessionTasks> | null | undefined): SessionTasks {
  return {
    ...(data?.plan ? { plan: data.plan } : {}),
    ...(data?.contract ? { contract: normalizeTaskContract(data.contract) } : {}),
    tasks: Array.isArray(data?.tasks)
      ? data.tasks.map((task) => ({
        ...task,
        evidence: Array.isArray(task.evidence) ? task.evidence.map(normalizeTaskEvidence) : [],
      }))
      : [],
  }
}

function normalizeTaskEvidence(data: Partial<TaskEvidence> | null | undefined): TaskEvidence {
  return {
    id: data?.id ?? '',
    type: data?.type ?? 'command_output_summary',
    ...(data?.title ? { title: data.title } : {}),
    ...(data?.summary ? { summary: data.summary } : {}),
    ...(data?.url ? { url: data.url } : {}),
    ...(data?.command ? { command: data.command } : {}),
    ...(data?.path ? { path: data.path } : {}),
    ...(data?.status ? { status: data.status } : {}),
    ...(data?.created_at ? { created_at: data.created_at } : {}),
  }
}

function normalizeTaskContract(data: Partial<TaskContract> | null | undefined): TaskContract {
  return {
    ...data,
    done_criteria: Array.isArray(data?.done_criteria) ? data.done_criteria : [],
    verification_commands: Array.isArray(data?.verification_commands) ? data.verification_commands : [],
    artifacts: Array.isArray(data?.artifacts) ? data.artifacts : [],
  }
}

function normalizePlanArchive(data: Partial<PlanArchiveResponse> | null | undefined): PlanArchiveResponse {
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    count: typeof data?.count === 'number' ? data.count : Array.isArray(data?.items) ? data.items.length : 0,
  }
}

function normalizeGlobalPlans(data: Partial<GlobalPlansResponse> | null | undefined): GlobalPlansResponse {
  return {
    items: Array.isArray(data?.items)
      ? data.items.map((item) => ({
        ...item,
        ...(item.contract ? { contract: normalizeTaskContract(item.contract) } : {}),
      }))
      : [],
    count: typeof data?.count === 'number' ? data.count : Array.isArray(data?.items) ? data.items.length : 0,
  }
}

// --- Server status ---

// --- Onboarding signals ---

export async function getHealthz(): Promise<HealthzResponse> {
  return requestJSON<HealthzResponse>('/v1/healthz')
}

export async function getSetupStatus(): Promise<SetupStatusResponse> {
  return requestJSON<SetupStatusResponse>('/v1/setup/status')
}

export async function getServerStatus(): Promise<{ version: string }> {
  return requestJSON<{ version: string }>('/v1/status')
}

export async function getTodayUsage(): Promise<UsageToday> {
  return requestJSON<UsageToday>('/v1/admin/usage/today')
}

export type LogsQuery = {
  file?: string
  level?: string
  component?: string
  lines?: number
}

export async function getLogs(query: LogsQuery = {}): Promise<LogsResponse> {
  const params = new URLSearchParams()
  if (query.file?.trim()) params.set('file', query.file.trim())
  if (query.level?.trim()) params.set('level', query.level.trim())
  if (query.component?.trim()) params.set('component', query.component.trim())
  if (query.lines && Number.isFinite(query.lines)) params.set('lines', String(query.lines))
  const suffix = params.toString()
  return requestJSON<LogsResponse>(`/v1/admin/logs${suffix ? `?${suffix}` : ''}`)
}

export async function getAnalytics(days = 7): Promise<AnalyticsResponse> {
  return requestJSON<AnalyticsResponse>(`/v1/admin/analytics?days=${days}`)
}

// --- Pulse (system watchdog, replaces heartbeat) ---

export async function getPulseStatus(): Promise<PulseSnapshot> {
  return requestJSON<PulseSnapshot>('/v1/pulse/status')
}

export async function runPulseOnce(): Promise<PulseTickOutcome> {
  return requestJSON<PulseTickOutcome>('/v1/pulse/run-once', { method: 'POST' })
}

export async function getPulseConfig(): Promise<PulseConfigView> {
  return requestJSON<PulseConfigView>('/v1/pulse/config')
}

// --- Reflection (nightly batch runner) ---

export async function getReflectionStatus(): Promise<ReflectionSnapshot> {
  return requestJSON<ReflectionSnapshot>('/v1/reflection/status')
}

export async function runReflectionOnce(): Promise<ReflectionRunSummary> {
  return requestJSON<ReflectionRunSummary>('/v1/reflection/run-once', { method: 'POST' })
}

export async function getReflectionConfig(): Promise<ReflectionConfigView> {
  return requestJSON<ReflectionConfigView>('/v1/reflection/config')
}

export async function listCronJobs(): Promise<CronJob[]> {
  return requestJSON<CronJob[]>('/v1/cron/jobs')
}

export async function listCronRuns(jobId: string, limit = 5): Promise<CronRunRecord[]> {
  return requestJSON<CronRunRecord[]>(`/v1/cron/jobs/${encodeURIComponent(jobId)}/runs?limit=${limit}`)
}

export async function getOpsStatus(): Promise<OpsStatus> {
  return requestJSON<OpsStatus>('/v1/ops/status')
}

export async function createCleanupPlan(): Promise<CleanupPlan> {
  return requestJSON<CleanupPlan>('/v1/ops/cleanup/plan', { method: 'POST' })
}

export async function applyCleanup(approvalId: string): Promise<CleanupApplyResult> {
  return requestJSON<CleanupApplyResult>('/v1/ops/cleanup/apply', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ approval_id: approvalId }),
  })
}

export async function listApprovals(): Promise<Approval[]> {
  return requestJSON<Approval[]>('/v1/ops/approvals')
}

export async function reviewApproval(approvalId: string, action: 'approve' | 'reject'): Promise<void> {
  await requestJSON<{ ok: boolean }>(`/v1/ops/approvals/${encodeURIComponent(approvalId)}/${action}`, {
    method: 'POST',
  })
}

export async function listAutomationAudit(limit = 50, sessionId = ''): Promise<AutomationAuditListResponse> {
  const params = new URLSearchParams()
  if (limit > 0) params.set('limit', String(limit))
  if (sessionId.trim()) params.set('session_id', sessionId.trim())
  const suffix = params.toString()
  return requestJSON<AutomationAuditListResponse>(`/v1/ops/automation-audit${suffix ? `?${suffix}` : ''}`)
}

export async function listSessions(includeHidden = false): Promise<Session[]> {
  const params = includeHidden ? '?hidden=1' : ''
  return requestJSON<Session[]>(`/v1/admin/sessions${params}`)
}

export async function createSession(title?: string): Promise<Session> {
  return requestJSON<Session>('/v1/admin/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: title || 'New Chat' }),
  })
}

export async function getSession(sessionId: string): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`)
}

export async function getSessionHistory(sessionId: string): Promise<SessionMessage[]> {
  return requestJSON<SessionMessage[]>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/history`)
}

export async function forkSessionFromMessage(sessionId: string, messageId: string, forkReason?: string): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/fork`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message_id: messageId, fork_reason: forkReason || 'Forked from chat transcript' }),
  })
}

export async function getForkPromotions(sessionId: string): Promise<ForkPromotionListResponse> {
  return requestJSON<ForkPromotionListResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/promotions`)
}

export async function promoteForkInsights(sessionId: string, candidateIds: string[]): Promise<ForkPromotionResult> {
  return requestJSON<ForkPromotionResult>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/promotions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ candidate_ids: candidateIds }),
  })
}

export async function renameSession(sessionId: string, title: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export async function deleteSession(sessionId: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'DELETE',
  })
}

export type AgentRuntimeRunsOptions = {
  limit?: number
  status?: string
  since?: string
  search?: string
}

export async function listAgentRuntimeRuns(options: number | AgentRuntimeRunsOptions = 30): Promise<AgentRuntimeRun[]> {
  const params = new URLSearchParams()
  const opts = typeof options === 'number' ? { limit: options } : options
  params.set('limit', String(opts.limit ?? 30))
  if (opts.status?.trim() && opts.status !== 'all') params.set('status', opts.status.trim())
  if (opts.since?.trim() && opts.since !== 'all') params.set('since', opts.since.trim())
  if (opts.search?.trim()) params.set('search', opts.search.trim())
  const payload = await requestJSON<{ runs: AgentRuntimeRun[] }>(`/v1/agentruntime/runs?${params.toString()}`)
  return payload.runs ?? []
}

export async function getAgentRuntimeRun(runId: string): Promise<AgentRuntimeRun> {
	return requestJSON<AgentRuntimeRun>(`/v1/agentruntime/runs/${encodeURIComponent(runId)}`)
}

export type AgentRuntimeRestartRequest = {
	checkpoint_id?: string
	agent?: string
	tier?: string
	provider_override?: AgentRuntimeProviderOverride
	prompt_adjustment?: string
	title?: string
}

export async function restartAgentRuntimeRun(runId: string, payload: AgentRuntimeRestartRequest): Promise<AgentRuntimeRun> {
	return requestJSON<AgentRuntimeRun>(`/v1/agentruntime/runs/${encodeURIComponent(runId)}/restart`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export async function listAgentRuntimeSubagents(): Promise<AgentRuntimeSubagentsResponse> {
	return requestJSON<AgentRuntimeSubagentsResponse>('/v1/agentruntime/subagents')
}

export async function getAgentRuntimeSubagent(name: string): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}`)
}

export async function updateAgentRuntimeSubagentTier(name: string, defaultTier: string): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ default_tier: defaultTier }),
	})
}

export async function draftAgentRuntimeSubagent(payload: {
	mode: 'create' | 'edit'
	request: string
	base_name?: string
	default_tier?: string
	use_llm?: boolean
}): Promise<AgentRuntimeSubagentDraftResponse> {
	return requestJSON<AgentRuntimeSubagentDraftResponse>('/v1/agentruntime/subagents/builder/draft', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export async function applyAgentRuntimeSubagentDraft(draft: AgentRuntimeSubagentDraft): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>('/v1/agentruntime/subagents/builder/apply', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ draft }),
	})
}

export async function archiveAgentRuntimeSubagent(name: string, confirm: boolean): Promise<AgentRuntimeSubagentArchiveResponse> {
	return requestJSON<AgentRuntimeSubagentArchiveResponse>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}/archive`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ confirm }),
	})
}

export async function recommendAgentRuntimeSubagents(payload: {
	limit?: number
	min_runs?: number
	include_failed?: boolean
} = {}): Promise<AgentRuntimeSubagentRecommendationsResponse> {
	return requestJSON<AgentRuntimeSubagentRecommendationsResponse>('/v1/agentruntime/subagents/recommendations', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export function streamAgentRuntimeRunEvents(
	runId: string,
	onEvent: (event: AgentRuntimeRunEvent) => void,
	onError?: (message: string) => void,
	onOpen?: () => void,
): () => void {
	const stream = new EventSource(`/v1/agentruntime/runs/${encodeURIComponent(runId)}/events`)
	stream.onopen = () => {
		onOpen?.()
	}
	stream.onmessage = (message) => {
		if (!message.data) return
		try {
			onEvent(JSON.parse(message.data) as AgentRuntimeRunEvent)
		} catch {
			onError?.('Failed to parse agent runtime run event')
		}
	}
	stream.onerror = () => onError?.('Agent Runtime run event stream disconnected')
	return () => stream.close()
}

export interface CompactResult {
  session_id: string
  compacted: boolean
  original_count: number
  final_count: number
  compacted_count: number
  tokens_before: number
  tokens_after: number
  reason: string
}

export async function compactSession(sessionId: string): Promise<CompactResult> {
  return requestJSON<CompactResult>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/compact`,
    { method: 'POST' },
  )
}

export async function getSessionTasks(sessionId: string): Promise<SessionTasks> {
  const data = await requestJSON<Partial<SessionTasks>>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks`)
  return normalizeSessionTasks(data)
}

export async function getGlobalPlans(active = true): Promise<GlobalPlansResponse> {
  const endpoint = active ? '/v1/admin/tasks?active=true' : '/v1/admin/tasks?active=false'
  const data = await requestJSON<Partial<GlobalPlansResponse>>(endpoint)
  return normalizeGlobalPlans(data)
}

export async function getPlanArchive(limit = 50): Promise<PlanArchiveResponse> {
  const data = await requestJSON<Partial<PlanArchiveResponse>>(`/v1/admin/plans/archive?limit=${limit}`)
  return normalizePlanArchive(data)
}

export async function getSessionPlanArchive(sessionId: string, limit = 20): Promise<PlanArchiveResponse> {
  const data = await requestJSON<Partial<PlanArchiveResponse>>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/plans/archive?limit=${limit}`,
  )
  return normalizePlanArchive(data)
}

// executeTasksAction drives the plan state machine directly from the
// console — used by the TasksPanel CTA buttons (Approve / Discard / Save
// edits). The body shape mirrors the chat-side `tasks` tool action.
export async function executeTasksAction(
  sessionId: string,
  payload: Record<string, unknown>,
): Promise<unknown> {
  return requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

// --- Git Inspector ---

type GitQuery = {
  sessionId?: string
  root?: string
}

function gitQueryParams(query: GitQuery = {}): URLSearchParams {
  const params = new URLSearchParams()
  if (query.sessionId?.trim()) params.set('session_id', query.sessionId.trim())
  if (query.root?.trim()) params.set('root', query.root.trim())
  return params
}

function gitEndpoint(path: string, query: GitQuery = {}): string {
  const params = gitQueryParams(query)
  const suffix = params.toString()
  return `/v1/git/${path}${suffix ? `?${suffix}` : ''}`
}

export function getGitStatus(query: GitQuery = {}): Promise<GitStatus> {
  return requestJSON<GitStatus>(gitEndpoint('status', query))
}

export function getGitDiff(query: GitQuery & { path?: string; staged?: boolean; hash?: string } = {}): Promise<GitDiff> {
  const params = gitQueryParams(query)
  if (query.path?.trim()) params.set('path', query.path.trim())
  if (query.staged) params.set('staged', '1')
  if (query.hash?.trim()) params.set('hash', query.hash.trim())
  const suffix = params.toString()
  return requestJSON<GitDiff>(`/v1/git/diff${suffix ? `?${suffix}` : ''}`)
}

export function getGitCommit(query: GitQuery & { hash: string }): Promise<GitCommitDetail> {
  const params = gitQueryParams(query)
  params.set('hash', query.hash.trim())
  return requestJSON<GitCommitDetail>(`/v1/git/commit?${params.toString()}`)
}

export function getGitWorktrees(query: GitQuery = {}): Promise<GitWorktreesResponse> {
  return requestJSON<GitWorktreesResponse>(gitEndpoint('worktrees', query))
}

export function getGitLog(query: GitQuery & { limit?: number } = {}): Promise<GitLogResponse> {
  const params = gitQueryParams(query)
  if (query.limit) params.set('limit', String(query.limit))
  const suffix = params.toString()
  return requestJSON<GitLogResponse>(`/v1/git/log${suffix ? `?${suffix}` : ''}`)
}

export function getGitBranches(query: GitQuery = {}): Promise<GitBranchesResponse> {
  return requestJSON<GitBranchesResponse>(gitEndpoint('branches', query))
}

export type CreateGitMutationApprovalRequest = {
  session_id: string
  root?: string
  action:
    | 'stage'
    | 'unstage'
    | 'discard'
    | 'commit'
    | 'switch_branch'
    | 'checkout_commit'
    | 'worktree_add'
    | 'worktree_remove'
    | 'fetch'
  path?: string
  branch?: string
  message?: string
  hash?: string
  worktree_path?: string
  new_branch?: string
  reason?: string
}

export function createGitMutationApproval(payload: CreateGitMutationApprovalRequest): Promise<GitMutationPlan> {
  return requestJSON<GitMutationPlan>('/v1/git/mutations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function getSessionWorkDirs(sessionId: string): Promise<SessionWorkDirs> {
  return requestJSON<SessionWorkDirs>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/workdirs`)
}

export async function updateSessionWorkDirs(sessionId: string, data: { work_dirs: string[]; current_dir: string }): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/workdirs`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function getSessionCwd(sessionId: string): Promise<SessionCwd> {
  return requestJSON<SessionCwd>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/cwd`)
}

export async function setSessionCwd(sessionId: string, current: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/cwd`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current }),
  })
}

export async function getSessionEffectiveConfig(sessionId: string): Promise<SessionEffectiveConfig> {
  return requestJSON<SessionEffectiveConfig>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/effective-config`,
  )
}

export type OpenTerminalResult = {
  ok: boolean
  cwd: string
  app: string
  message?: string
}

export async function openTerminalHere(sessionId: string, cwd?: string): Promise<OpenTerminalResult> {
  return requestJSON<OpenTerminalResult>('/v1/terminal/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, cwd: cwd || '' }),
  })
}

export function terminalWebSocketURL(sessionId: string, cwd?: string, cols = 80, rows = 24): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const params = new URLSearchParams({
    session_id: sessionId,
    cols: String(cols),
    rows: String(rows),
  })
  if (cwd) params.set('cwd', cwd)
  return `${protocol}//${window.location.host}/v1/terminal/ws?${params}`
}

export async function listMemoryAssets(): Promise<{ count: number; items: MemoryAsset[] }> {
  return requestJSON<{ count: number; items: MemoryAsset[] }>('/v1/memory/assets')
}

export async function getMemoryFile(path: string): Promise<MemoryFile> {
  return requestJSON<MemoryFile>(`/v1/memory/file?path=${encodeURIComponent(path)}`)
}

export async function saveMemoryFile(path: string, content: string): Promise<MemoryFile> {
  return requestJSON<MemoryFile>('/v1/memory/file', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

export async function listMemoryInbox(status: MemoryCandidateStatus | 'all' = 'pending'): Promise<MemoryCandidateListResponse> {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  const qs = params.toString()
  return requestJSON<MemoryCandidateListResponse>(`/v1/memory/inbox${qs ? `?${qs}` : ''}`)
}

export async function reviewMemoryCandidate(
  id: string,
  action: MemoryCandidateAction,
  mergeTarget?: string,
): Promise<MemoryCandidateReviewResponse> {
  return requestJSON<MemoryCandidateReviewResponse>('/v1/memory/inbox/review', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, action, merge_target: mergeTarget || '' }),
  })
}

export async function runMemorySearch(payload: {
  query: string
  limit?: number
  include_memory?: boolean
  include_daily?: boolean
  include_sessions?: boolean
}): Promise<MemorySearchResult> {
  return requestJSON<MemorySearchResult>('/v1/memory/search', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function runMemoryPrefetch(payload: {
  query: string
  session_id?: string
}): Promise<MemoryPrefetchResult> {
  const params = new URLSearchParams({ query: payload.query })
  if (payload.session_id?.trim()) params.set('session_id', payload.session_id.trim())
  return requestJSON<MemoryPrefetchResult>(`/v1/memory/prefetch?${params.toString()}`, {
    method: 'POST',
  })
}

export async function listSyspromptFiles(scope?: 'workspace' | 'agent'): Promise<{ count: number; items: SyspromptFile[] }> {
  const qs = scope ? `?scope=${encodeURIComponent(scope)}` : ''
  return requestJSON<{ count: number; items: SyspromptFile[] }>(`/v1/workspace/sysprompt/files${qs}`)
}

export async function getSyspromptFile(scope: 'workspace' | 'agent', path: string): Promise<SyspromptFile> {
  return requestJSON<SyspromptFile>(`/v1/workspace/sysprompt/file?scope=${encodeURIComponent(scope)}&path=${encodeURIComponent(path)}`)
}

export async function saveSyspromptFile(scope: 'workspace' | 'agent', path: string, content: string): Promise<SyspromptFile> {
  return requestJSON<SyspromptFile>('/v1/workspace/sysprompt/file', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ scope, path, content }),
  })
}

export async function getSyspromptPreview(target: 'main_agent' | 'sub_agent' = 'main_agent'): Promise<SyspromptPreview> {
  return requestJSON<SyspromptPreview>(`/v1/admin/sysprompt/preview?target=${encodeURIComponent(target)}`)
}

// --- Session Config ---

export type SessionToolConfig = {
  tools_enabled?: string[]
  tools_custom?: boolean
  tools_disabled?: string[]
  tools_allow_groups?: string[]
  tools_deny_groups?: string[]
  skills_enabled?: string[]
  skills_custom?: boolean
  mcp_enabled?: string[]
}

export async function getSessionConfig(sessionId: string): Promise<SessionToolConfig> {
  return requestJSON<SessionToolConfig>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/config`)
}

export async function updateSessionConfig(sessionId: string, config: SessionToolConfig): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/config`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
}

export async function getSessionAutomationConsent(sessionId: string): Promise<SessionAutomationConsent> {
  return requestJSON<SessionAutomationConsent>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/automation-consent`)
}

export async function updateSessionAutomationConsent(
  sessionId: string,
  consent: SessionAutomationConsent,
): Promise<SessionAutomationConsent> {
  return requestJSON<SessionAutomationConsent>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/automation-consent`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(consent),
  })
}

export async function getSessionStyle(sessionId: string): Promise<SessionStyleResponse> {
  return requestJSON<SessionStyleResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/style`)
}

export async function updateSessionStyle(
  sessionId: string,
  style: SessionStyleControl,
): Promise<SessionStyleResponse> {
  return requestJSON<SessionStyleResponse>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/style`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(style),
  })
}

export type ChatToolInfo = {
  name: string
  description: string
  high_risk: boolean
  group?: string
}

export type ChatToolsResponse = {
  tools: ChatToolInfo[]
  skills?: string[]
  mcp_servers?: string[]
}

export async function listChatTools(): Promise<ChatToolsResponse> {
  return requestJSON<ChatToolsResponse>('/v1/chat/tools')
}

// --- Chat Context ---

export type ChatContextInfo = {
  session_id: string
  system_prompt: string
  system_prompt_tokens: number
  history_tokens: number
  history_messages: number
  tool_count: number
  tool_names: string[]
  skill_count?: number
  skill_names?: string[]
  memory_count: number
  memory_tokens: number
  compaction_trigger_tokens?: number
  compaction_keep_recent_tokens?: number
  compaction_keep_recent_fraction?: number
  compaction_last_mode?: string
  used_tool_names?: string[]
  selected_skill_name?: string
  selected_skill_reason?: string
  mentioned_path_count?: number
  mentioned_paths?: string[]
  mentioned_subagent_count?: number
  mentioned_subagents?: string[]
  llm_tier?: string
  tier_recommendation?: ChatTierRecommendationRequest
  style_effective?: SessionStyleResponse['effective']
  prompt_override: string
}

export async function getChatContext(sessionId: string): Promise<ChatContextInfo> {
  return requestJSON<ChatContextInfo>(`/v1/chat/context?session_id=${encodeURIComponent(sessionId)}`)
}

export type PriorContextPreviewItem = {
  source: string
  source_tag: string
  snippet: string
  tokens: number
}

export type PriorContextPreviewMode = 'default' | 'recent'

export type PriorContextPreview = {
  session_id: string
  query: string
  mode: PriorContextPreviewMode
  section: string
  items: PriorContextPreviewItem[]
  below_threshold_items: PriorContextPreviewItem[]
  recent_fallback_items: PriorContextPreviewItem[]
  relevant_tokens: number
  relevant_memory_count: number
  relevant_budget_tokens: number
  budget_percent: number
  generated_at: string
}

export async function getPriorContextPreview(sessionId: string, query: string): Promise<PriorContextPreview> {
  return requestJSON<PriorContextPreview>('/v1/chat/prior-context/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, query }),
  })
}

export type ChatFileMentionCandidate = {
  kind: 'file' | 'directory'
  name: string
  path: string
  root: string
  root_label: string
  token: string
  size?: number
  updated_at?: string
}

export async function listChatFileMentions(
  sessionId: string | undefined,
  query: string,
  limit = 30,
): Promise<{ candidates: ChatFileMentionCandidate[] }> {
  const params = new URLSearchParams({ q: query, limit: String(limit) })
  if (sessionId?.trim()) params.set('session_id', sessionId.trim())
  return requestJSON<{ candidates: ChatFileMentionCandidate[] }>(
    `/v1/chat/mentions/files?${params}`,
  )
}

export async function getSessionPrompt(sessionId: string): Promise<{ prompt_override: string }> {
  return requestJSON<{ prompt_override: string }>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/prompt`)
}

export async function updateSessionPrompt(sessionId: string, promptOverride: string): Promise<void> {
  await requestJSON(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/prompt`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt_override: promptOverride }),
  })
}

export async function getEventsHistory(limit = 30): Promise<EventsHistoryInfo> {
  return requestJSON<EventsHistoryInfo>(`/v1/events/history?limit=${limit}`)
}

export async function markEventsRead(lastId: number): Promise<{ unread_count: number }> {
  return requestJSON<{ acknowledged: boolean; read_cursor: number; unread_count: number }>('/v1/events/read', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ last_id: lastId }),
  })
}

// --- Cron Job CRUD ---

export async function createCronJob(data: CreateCronJobRequest): Promise<CronJob> {
  return requestJSON<CronJob>('/v1/cron/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function updateCronJob(jobId: string, data: UpdateCronJobRequest): Promise<CronJob> {
  return requestJSON<CronJob>(`/v1/cron/jobs/${encodeURIComponent(jobId)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function deleteCronJob(jobId: string): Promise<void> {
  await requestJSON<Record<string, never>>(`/v1/cron/jobs/${encodeURIComponent(jobId)}`, {
    method: 'DELETE',
  })
}

export async function runCronJob(jobId: string): Promise<CronRunResult> {
  return requestJSON<CronRunResult>(`/v1/cron/jobs/${encodeURIComponent(jobId)}/run`, {
    method: 'POST',
  })
}

// --- Config ---

export async function getConfig(): Promise<ConfigFile> {
  return requestJSON<ConfigFile>('/v1/admin/config')
}

export async function getConfigSchema(): Promise<ConfigSchema> {
  return requestJSON<ConfigSchema>('/v1/admin/config/schema')
}

export async function getProviderModels(providerAlias = ''): Promise<ProviderModelsInfo> {
  const params = new URLSearchParams()
  if (providerAlias.trim()) {
    params.set('provider_alias', providerAlias.trim())
  }
  const suffix = params.toString()
  return requestJSON<ProviderModelsInfo>(`/v1/models${suffix ? `?${suffix}` : ''}`)
}

export async function getProviders(): Promise<ProvidersAPIInfo> {
  return requestJSON<ProvidersAPIInfo>('/v1/providers')
}

export async function saveConfig(content: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/admin/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
}

export async function restartServer(): Promise<{ ok: string; mode: string; info: string }> {
  return requestJSON<{ ok: string; mode: string; info: string }>('/v1/admin/restart', { method: 'POST' })
}

export type WorkspaceResetResponse = {
  removed: number
  removed_items: string[]
  failed_items?: { name?: string; path?: string; stage?: string; error: string }[]
  reinitialized: boolean
  error?: string
}

export async function resetWorkspace(): Promise<WorkspaceResetResponse> {
  return requestJSON<WorkspaceResetResponse>('/v1/admin/reset/workspace', { method: 'POST' })
}

export async function patchConfigValues(updates: Record<string, unknown>): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/admin/config/values', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ updates }),
  })
}

// --- Channels / Telegram ---

export async function getTelegramPairings(): Promise<TelegramPairingsResponse> {
  return requestJSON<TelegramPairingsResponse>('/v1/channels/telegram/pairings')
}

export async function approveTelegramPairing(code: string): Promise<{ approved: TelegramAllowedUser }> {
  return requestJSON<{ approved: TelegramAllowedUser }>('/v1/channels/telegram/pairings/approve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: code.trim() }),
  })
}

export async function revokeTelegramPairing(userId: number): Promise<{ revoked: boolean }> {
  return requestJSON<{ revoked: boolean }>('/v1/channels/telegram/pairings/revoke', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId }),
  })
}

// --- Hub / Extensions ---

export async function getHubRegistry(): Promise<HubRegistry> {
  return requestJSON<HubRegistry>('/v1/hub/registry')
}

export async function getHubInstalled(): Promise<HubInstalled> {
  return requestJSON<HubInstalled>('/v1/hub/installed')
}

export async function hubInstall(type: string, name: string): Promise<HubInstallResponse> {
  return requestJSON<HubInstallResponse>('/v1/hub/install', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type, name }),
  })
}

export async function hubUninstall(type: string, name: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/hub/uninstall', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type, name }),
  })
}

export async function hubUpdate(): Promise<{ updated_skills: string[]; updated_plugins: string[] }> {
  return requestJSON<{ updated_skills: string[]; updated_plugins: string[] }>('/v1/hub/update', { method: 'POST' })
}

export async function listSkills(): Promise<SkillDef[]> {
  return requestJSON<SkillDef[]>('/v1/skills')
}

export async function listPlugins(): Promise<PluginDef[]> {
  return requestJSON<PluginDef[]>('/v1/plugins')
}

export async function listMCPServers(): Promise<MCPServerStatus[]> {
  return requestJSON<MCPServerStatus[]>('/v1/mcp/servers')
}

export async function getDisabledExtensions(): Promise<{ skills: string[]; plugins: string[]; mcp_servers: string[] }> {
  return requestJSON('/v1/runtime/extensions/disabled')
}

export async function setExtensionDisabled(kind: string, name: string, disabled: boolean): Promise<void> {
  await requestJSON<{ ok: boolean }>('/v1/runtime/extensions/disabled', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, name, disabled }),
  })
}

export async function getSkillDetail(name: string): Promise<SkillDef & { content?: string }> {
  return requestJSON<SkillDef & { content?: string }>(`/v1/skills/${encodeURIComponent(name)}`)
}

export async function getHubSkillContent(name: string): Promise<{ name: string; version: string; content: string }> {
  return requestJSON<{ name: string; version: string; content: string }>(`/v1/hub/skill-content?name=${encodeURIComponent(name)}`)
}

export async function reloadExtensions(): Promise<{ reloaded: boolean; skills: number; plugins: number; mcp_count: number }> {
  return requestJSON<{ reloaded: boolean; skills: number; plugins: number; mcp_count: number }>('/v1/runtime/extensions/reload', { method: 'POST' })
}

export async function draftSkill(payload: SkillCreatorDraftRequest): Promise<SkillCreatorDraftResponse> {
  return requestJSON<SkillCreatorDraftResponse>('/v1/admin/skills/draft', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function saveLocalSkill(draft: SkillCreatorDraftResponse): Promise<SkillCreatorSaveResponse> {
  return requestJSON<SkillCreatorSaveResponse>('/v1/admin/skills/save-local', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function testSkillDraft(draft: SkillCreatorDraftResponse): Promise<SkillCreatorTestResponse> {
  return requestJSON<SkillCreatorTestResponse>('/v1/admin/skills/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function submitSkillDraftPR(name: string): Promise<SkillCreatorSubmitResponse> {
  return requestJSON<SkillCreatorSubmitResponse>('/v1/admin/skills/submit-pr', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export async function listSkillExtractions(status: SkillExtractionCandidateStatus | 'all' = 'pending'): Promise<SkillExtractionListResponse> {
  const qs = new URLSearchParams()
  if (status) qs.set('status', status)
  return requestJSON<SkillExtractionListResponse>(`/v1/admin/skills/extractions?${qs}`)
}

export async function extractSkillsFromSession(sessionId: string, maxCandidates = 5): Promise<SkillExtractionListResponse> {
  return requestJSON<SkillExtractionListResponse>('/v1/admin/skills/extractions/extract', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, max_candidates: maxCandidates }),
  })
}

export async function reviewSkillExtractionCandidate(id: string, action: SkillExtractionCandidateAction): Promise<SkillExtractionReviewResponse> {
  return requestJSON<SkillExtractionReviewResponse>('/v1/admin/skills/extractions/review', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, action }),
  })
}

export async function draftMCPServer(payload: MCPServerCreatorDraftRequest): Promise<MCPServerCreatorDraftResponse> {
  return requestJSON<MCPServerCreatorDraftResponse>('/v1/admin/mcp-servers/draft', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function saveLocalMCPServer(draft: MCPServerCreatorDraftResponse): Promise<MCPServerCreatorSaveResponse> {
  return requestJSON<MCPServerCreatorSaveResponse>('/v1/admin/mcp-servers/save-local', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function testMCPServerDraft(draft: MCPServerCreatorDraftResponse): Promise<MCPServerCreatorTestResponse> {
  return requestJSON<MCPServerCreatorTestResponse>('/v1/admin/mcp-servers/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function submitMCPServerDraftPR(name: string): Promise<MCPServerCreatorSubmitResponse> {
  return requestJSON<MCPServerCreatorSubmitResponse>('/v1/admin/mcp-servers/submit-pr', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

// --- Events (singleton SSE) ---
//
// A single EventSource is shared across all components to avoid exhausting the
// browser's per-origin HTTP/1.1 connection limit (typically 6).  Components
// call streamEvents() to subscribe and receive a cleanup function that
// unsubscribes without closing the underlying connection.

type EventListener = {
  onEvent: (event: NotificationMessage) => void
  onError?: (message: string) => void
  onOpen?: () => void
  onReopen?: () => void
}

let sharedStream: EventSource | null = null
let listeners = new Map<number, EventListener>()
let nextListenerId = 0
let hasOpenedOnce = false
let reconnectAttempt = 0
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

function clearReconnectTimer() {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function scheduleReconnect() {
  if (reconnectTimer !== null) return
  if (listeners.size === 0) return
  // Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s.
  const delay = Math.min(30_000, 1_000 * 2 ** Math.min(reconnectAttempt, 5))
  reconnectAttempt += 1
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    if (listeners.size === 0) return
    ensureStream()
  }, delay)
}

function ensureStream() {
  if (sharedStream && sharedStream.readyState !== EventSource.CLOSED) return
  if (sharedStream) {
    sharedStream.close()
    sharedStream = null
  }
  const wasOpenedBefore = hasOpenedOnce
  sharedStream = new EventSource('/v1/events/stream')
  sharedStream.onopen = () => {
    clearReconnectTimer()
    reconnectAttempt = 0
    const isReopen = wasOpenedBefore
    hasOpenedOnce = true
    for (const l of listeners.values()) {
      l.onOpen?.()
      if (isReopen) l.onReopen?.()
    }
  }
  sharedStream.onmessage = (message) => {
    if (!message.data) return
    try {
      const payload = JSON.parse(message.data) as NotificationMessage
      if (payload.type === 'keepalive') return
      for (const l of listeners.values()) l.onEvent(payload)
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Failed to parse event stream payload'
      for (const l of listeners.values()) l.onError?.(msg)
    }
  }
  sharedStream.onerror = () => {
    for (const l of listeners.values()) l.onError?.('Event stream disconnected')
    // EventSource auto-reconnects on transient drops (readyState=CONNECTING).
    // Only schedule a manual reconnect once it gives up and reaches CLOSED.
    if (sharedStream?.readyState === EventSource.CLOSED) {
      scheduleReconnect()
    }
  }
}

function maybeCloseStream() {
  if (listeners.size === 0 && sharedStream) {
    sharedStream.close()
    sharedStream = null
    clearReconnectTimer()
    reconnectAttempt = 0
    hasOpenedOnce = false
  }
}

export function streamEvents(
  onEvent: (event: NotificationMessage) => void,
  onError?: (message: string) => void,
  onOpen?: () => void,
  onReopen?: () => void,
): () => void {
  const id = nextListenerId++
  listeners.set(id, { onEvent, onError, onOpen, onReopen })
  ensureStream()
  return () => {
    listeners.delete(id)
    maybeCloseStream()
  }
}

export async function streamChat(
  request: ChatRequest,
  onEvent: (event: ChatEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const response = await fetch('/v1/chat', {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
    signal,
  })

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim()
    try {
      const payload = (await response.json()) as APIErrorPayload
      if (payload?.error?.trim()) {
        message = payload.error.trim()
      }
    } catch {
      // ignore non-JSON error bodies
    }
    throw new Error(message)
  }

  if (!response.body) {
    throw new Error('chat stream body missing')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() ?? ''

    for (const line of lines) {
      if (!line.startsWith('data:')) {
        continue
      }
      const payload = line.slice(5).trim()
      if (!payload) {
        continue
      }
      onEvent(JSON.parse(payload) as ChatEvent)
    }

    if (done) {
      break
    }
  }
}

export async function cancelChat(sessionId: string): Promise<boolean> {
  try {
    const result = await requestJSON<{ cancelled: boolean }>(
      `/v1/chat/cancel?session_id=${encodeURIComponent(sessionId)}`,
      { method: 'POST' },
    )
    return result.cancelled
  } catch {
    return false
  }
}

// --- Workspace Files ---

export type WorkspaceFileEntry = {
  name: string
  path: string
  is_dir: boolean
  size?: number
  updated_at?: string
}

export type WorkspaceFileContent = {
  path: string
  name: string
  size: number
  updated_at: string
  kind: 'text' | 'markdown' | 'image' | 'binary'
  mime_type: string
  encoding?: 'utf-8' | 'base64'
  content?: string
  content_base64?: string
  truncated?: boolean
  is_binary?: boolean
  message?: string
}

function workspaceFilesEndpoint(root?: string): string {
  return root ? '/v1/filesystem/files' : '/v1/workspace/files'
}

export async function listWorkspaceFiles(path = '.', root?: string): Promise<{ path: string; files: WorkspaceFileEntry[] }> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<{ path: string; files: WorkspaceFileEntry[] }>(
    `${workspaceFilesEndpoint(root)}?${params}`
  )
}

export async function readWorkspaceFile(path: string, root?: string): Promise<WorkspaceFileContent> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<WorkspaceFileContent>(
    `${workspaceFilesEndpoint(root)}?${params}`
  )
}

export async function createWorkspaceDirectory(parentPath: string, name: string, root?: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  const params = new URLSearchParams()
  if (root) params.set('root', root)
  return requestJSON<{ path: string; name: string; is_dir: boolean }>(
    `${workspaceFilesEndpoint(root)}?${params}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ parent_path: parentPath, name }),
    },
  )
}

export async function renameWorkspaceDirectory(path: string, newName: string, root?: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  const params = new URLSearchParams()
  if (root) params.set('root', root)
  return requestJSON<{ path: string; name: string; is_dir: boolean }>(
    `${workspaceFilesEndpoint(root)}?${params}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, new_name: newName }),
    },
  )
}

// --- Filesystem ---

export type FilesystemBrowseResult = {
  path: string
  parent: string
  entries: { name: string; is_dir: boolean; is_git?: boolean }[]
}

export async function browseFilesystem(path?: string): Promise<FilesystemBrowseResult> {
  const params = path ? `?path=${encodeURIComponent(path)}` : ''
  return requestJSON(`/v1/filesystem/browse${params}`)
}

export async function createFilesystemDirectory(parentPath: string, name: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  return requestJSON('/v1/filesystem/browse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ parent_path: parentPath, name }),
  })
}
