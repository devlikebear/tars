// --- Pulse (system watchdog) ---

export type PulseDecision = {
  action: 'ignore' | 'notify' | 'autofix'
  severity: 'info' | 'warn' | 'error' | 'critical'
  title?: string
  summary?: string
  details?: Record<string, unknown>
  autofix_name?: string
}

export type PulseSignal = {
  kind: string
  severity: 'info' | 'warn' | 'error' | 'critical'
  summary: string
  details?: Record<string, unknown>
  at: string
}

export type PulseTickOutcome = {
  at: string
  skipped?: boolean
  skip_reason?: string
  signals?: PulseSignal[]
  decider_invoked?: boolean
  decision?: PulseDecision
  autofix_attempt?: string
  autofix_ok?: boolean
  autofix_err?: string
  notify_delivered?: boolean
  err?: string
}

export type PulseSnapshot = {
  last_tick_at: string
  last_decision?: PulseDecision
  last_err?: string
  total_ticks: number
  total_skipped: number
  total_decisions: number
  total_autofixes: number
  total_notifies: number
  recent: PulseTickOutcome[]
}

export type PulseConfigView = {
  enabled: boolean
  interval_seconds: number
  timeout_seconds: number
  active_hours: string
  timezone: string
  min_severity: string
  allowed_autofixes: string[]
  notify_telegram: boolean
  notify_session_events: boolean
  cron_failure_threshold: number
  stuck_run_minutes: number
  disk_warn_percent: number
  disk_critical_percent: number
}

export type UsageToday = {
  date: string
  reset_at: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
  budget_tokens: number
  budget_enabled: boolean
  usage_percent: number
  level: 'disabled' | 'default' | 'warning' | 'error'
}

export type LogFileOption = {
  id: string
  label: string
  path: string
  exists: boolean
  size_bytes: number
  updated_at?: string
}

export type LogLineView = {
  raw: string
  level?: string
  component?: string
  message?: string
  time?: string
  fields?: Record<string, unknown>
}

export type LogsResponse = {
  files: LogFileOption[]
  selected_file: string
  lines: LogLineView[]
  count: number
  lines_requested: number
  level: string
  component: string
}

export type AnalyticsTotals = {
  calls: number
  sessions: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cost_usd: number
  avg_tokens_per_session: number
}

export type AnalyticsDailyRow = {
  day: string
  calls: number
  sessions: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cost_usd: number
}

export type AnalyticsModelRow = {
  provider: string
  model: string
  calls: number
  sessions: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cost_usd: number
}

export type AnalyticsSkillRow = {
  name: string
  source?: string
  calls: number
  first_at?: string
  last_at?: string
}

export type AnalyticsResponse = {
  days: number
  totals: AnalyticsTotals
  daily: AnalyticsDailyRow[]
  models: AnalyticsModelRow[]
  skills: AnalyticsSkillRow[]
}

// --- Reflection (nightly batch runner) ---

export type ReflectionJobResult = {
  name: string
  success: boolean
  changed?: boolean
  summary?: string
  details?: Record<string, unknown>
  err?: string
  duration_ms: number
}

export type ReflectionRunSummary = {
  started_at: string
  finished_at: string
  results: ReflectionJobResult[]
  success: boolean
  err?: string
}

export type ReflectionSnapshot = {
  last_run_at: string
  last_run_success: boolean
  last_run_summary?: ReflectionRunSummary
  last_successful_run_at?: string
  consecutive_failures: number
  total_runs: number
  total_successes: number
  total_failures: number
  recent: ReflectionRunSummary[]
}

export type ReflectionConfigView = {
  enabled: boolean
  sleep_window: string
  timezone: string
  tick_interval_seconds: number
  empty_session_age_seconds: number
  memory_lookback_hours: number
  max_turns_per_session: number
}

export type CronJob = {
  id: string
  name: string
  prompt: string
  schedule: string
  enabled: boolean
  delete_after_run?: boolean
  session_id?: string
  session_target?: string
  wake_mode?: string
  delivery_mode?: string
  last_run_at?: string
  last_run_error?: string
}

export type CronRunRecord = {
  job_id: string
  ran_at: string
  response?: string
  error?: string
}

export type NotificationMessage = {
  id?: number
  type: string
  category: string
  severity: string
  title: string
  message: string
  timestamp: string
  job_id?: string
  session_id?: string
  open_path?: string
}

export type ProviderOverride = {
  alias?: string
  model?: string
}

export type ConsensusVariantRecord = {
  variant_idx: number
  alias?: string
  kind?: string
  model?: string
  status?: string
  response?: string
  error?: string
  tokens_in?: number
  tokens_out?: number
  cost_usd?: number
  started_at?: string
  finished_at?: string
}

export type FileAttentionSummary = {
  path: string
  total: number
  reads?: number
  edits?: number
  lists?: number
  writes?: number
  first_at?: string
  last_at?: string
  sparkline?: number[]
}

export type AgentRuntimeDiffSummary = {
  files: number
  additions: number
  deletions: number
}

export type AgentRuntimeDiffFileChange = {
  path: string
  status: string
  additions?: number
  deletions?: number
  patch?: string
  git_inspector_url?: string
}

export type AgentRuntimeDiffTimelineEntry = {
  id: string
  run_id: string
  session_id?: string
  session_kind?: string
  agent?: string
  prompt?: string
  parent_run_id?: string
  root_run_id?: string
  flow_id?: string
  step_id?: string
  started_at?: string
  completed_at?: string
  repo_root?: string
  git_inspector_url?: string
  summary: AgentRuntimeDiffSummary
  files?: AgentRuntimeDiffFileChange[]
}

export type AgentRuntimeRun = {
  run_id: string
  session_id?: string
  session_kind?: string
  agent?: string
  prompt?: string
  parent_run_id?: string
  root_run_id?: string
  parent_session_id?: string
  depth?: number
  status: string
  accepted?: boolean
  response?: string
  error?: string
  tier?: string
  resolved_alias?: string
  resolved_kind?: string
  resolved_model?: string
  override_source?: string
  consensus_mode?: string
  consensus_variants?: ConsensusVariantRecord[]
  consensus_cost_usd?: number
  consensus_budget_usd?: number
  file_attention?: FileAttentionSummary[]
  file_ops_total?: number
  diff_timeline?: AgentRuntimeDiffTimelineEntry[]
  created_at?: string
  started_at?: string
  completed_at?: string
  updated_at?: string
}

export type AgentRuntimeRunEvent = {
  type: string
  run_id: string
  timestamp?: string
  agent?: string
  status?: string
  tier?: string
  resolved_alias?: string
  resolved_kind?: string
  resolved_model?: string
  error?: string
  message?: string
  response?: string
  variant_count?: number
  variant_idx?: number
  alias?: string
  kind?: string
  model?: string
  strategy?: string
  token_budget?: number
  tokens_in?: number
  tokens_out?: number
  final_tokens?: number
  cost_usd_estimate?: number
  cost_usd_actual?: number
  tool_name?: string
  tool_call_id?: string
  path?: string
  action?: string
  tool_is_error?: boolean
}

export type AgentRuntimeTierOption = {
  name: string
  provider_alias?: string
  kind?: string
  model?: string
  reasoning_effort?: string
  thinking_budget?: number
  service_tier?: string
  error?: string
}

export type AgentRuntimeSubagentRunSummary = {
  run_id: string
  status: string
  tier?: string
  created_at?: string
  updated_at?: string
  completed_at?: string
}

export type AgentRuntimeProviderOverride = {
  alias?: string
  model?: string
}

export type AgentRuntimeSubagent = {
  name: string
  description?: string
  enabled: boolean
  kind?: string
  source?: string
  entry?: string
  default: boolean
  policy_mode?: string
  tools_allow: string[]
  tools_allow_count: number
  tools_deny: string[]
  tools_deny_count: number
  tools_risk_max?: string
  tools_allow_groups: string[]
  tools_deny_groups: string[]
  tools_allow_patterns: string[]
  session_routing_mode?: string
  session_fixed_id?: string
  default_tier?: string
  effective_tier?: string
  tier_source?: string
  tier_missing: boolean
  tier_error?: string
  tier_editable: boolean
  provider_override?: AgentRuntimeProviderOverride
  resolved_alias?: string
  resolved_kind?: string
  resolved_model?: string
  run_count: number
  last_run?: AgentRuntimeSubagentRunSummary
  recent_runs: AgentRuntimeSubagentRunSummary[]
}

export type AgentRuntimeSubagentDraft = {
  action: 'create' | 'update'
  name: string
  description: string
  default_tier: string
  prompt: string
  tools_allow: string[]
  tools_deny: string[]
  tools_risk_max?: string
  session_routing_mode?: string
  session_fixed_id?: string
}

export type AgentRuntimeSubagentDraftResponse = {
  draft: AgentRuntimeSubagentDraft
  draft_source: string
  warnings?: string[]
  tiers: AgentRuntimeTierOption[]
  resolved_tier?: AgentRuntimeTierOption
}

export type AgentRuntimeSubagentArchiveResponse = {
  archived: boolean
  name: string
  archived_path?: string
  run_count: number
}

export type AgentRuntimeSubagentsResponse = {
  count: number
  agents: AgentRuntimeSubagent[]
  tiers: AgentRuntimeTierOption[]
  default_tier?: string
  agentruntime_default_tier?: string
  agentruntime_default_tier_source?: string
}

export type EventsHistoryInfo = {
  items: NotificationMessage[]
  unread_count: number
  read_cursor: number
  last_id: number
}

export type Approval = {
  id: string
  type: string
  status: string
  requested_at: string
  updated_at: string
  reviewed_at?: string
  note?: string
  plan: {
    approval_id: string
    created_at: string
    total_bytes: number
    candidates: Array<{
      path: string
      size_bytes: number
      reason?: string
    }>
  }
}

export type Session = {
  id: string
  title: string
  kind?: string
  hidden?: boolean
  created_at: string
  updated_at: string
}

export type SessionMessage = {
  role: string
  content: string
  timestamp: string
  tool_name?: string
  tool_call_id?: string
  tool_args?: string
  tool_is_error?: boolean
}

export type OpsStatus = {
  timestamp: string
  disk_total_bytes: number
  disk_free_bytes: number
  disk_used_percent: number
  process_count: number
}

export type CleanupPlan = {
  approval_id: string
  created_at: string
  total_bytes: number
  candidates: Array<{
    path: string
    size_bytes: number
    reason?: string
  }>
}

export type CleanupApplyResult = {
  approval_id: string
  deleted_count: number
  deleted_bytes: number
  errors?: string[]
}

export type APIErrorPayload = {
  error?: string
  sandbox_report?: SkillSandboxReport
}

export type ChatEvent = {
  type: string
  text?: string
  error?: string
  session_id?: string
  message?: string
  phase?: string
  mode?: string
  tool_name?: string
  tool_call_id?: string
  tool_args_preview?: string
  tool_result_preview?: string
  tool_is_error?: boolean
  skill_name?: string
  skill_reason?: string
  // context_info fields
  system_prompt_tokens?: number
  history_tokens?: number
  history_messages?: number
  tool_count?: number
  tool_names?: string[]
  skill_count?: number
  skill_names?: string[]
  memory_count?: number
  memory_tokens?: number
  compaction_trigger_tokens?: number
  compaction_keep_recent_tokens?: number
  compaction_keep_recent_fraction?: number
  compaction_last_mode?: string
  original_count?: number
  final_count?: number
  compacted_count?: number
  trigger_tokens?: number
  estimated_tokens_before?: number
  used_tool_names?: string[]
  selected_skill_name?: string
  selected_skill_reason?: string
  mentioned_path_count?: number
  mentioned_paths?: string[]
  mentioned_subagent_count?: number
  mentioned_subagents?: string[]
  // tasks_changed event fields (live count for chat pulse-bar Tasks badge)
  task_total?: number
  task_pending?: number
  task_in_progress?: number
  task_completed?: number
  task_cancelled?: number
  plan_goal?: string
  // done event usage
  usage?: {
    input_tokens: number
    output_tokens: number
    cached_tokens: number
    cache_read_tokens: number
    cache_write_tokens: number
  }
}

export type ChatAttachment = {
  name: string
  mime_type: string
  data: string // base64
}

export type ChatFileMention = {
  kind: 'file' | 'directory'
  root: string
  path: string
}

export type ChatSubagentMention = {
  name: string
  token?: string
}

export type ChatRequest = {
  message: string
  session_id?: string
  attachments?: ChatAttachment[]
  mentions?: ChatFileMention[]
  subagent_mentions?: ChatSubagentMention[]
}

export type MemoryAsset = {
  path: string
  kind: string
  editable: boolean
  size_bytes: number
  updated_at?: string
}

export type MemoryFile = {
  path: string
  kind: string
  editable: boolean
  content: string
  size_bytes?: number
  updated_at?: string
}

export type MemoryCandidateStatus = 'pending' | 'approved' | 'rejected' | 'merged'

export type MemoryCandidateAction = 'approve' | 'reject' | 'merge'

export type MemoryCandidateProvenance = {
  source?: string
  session_id?: string
  message_range?: string
  source_summary?: string
  extracted_at?: string
}

export type MemoryCandidateHint = {
  kind: 'similar' | 'conflict' | string
  category: string
  summary: string
  source_session?: string
  score?: number
  reason?: string
}

export type MemoryCandidate = {
  id: string
  status: MemoryCandidateStatus
  category: string
  summary: string
  tags?: string[]
  source_session?: string
  importance?: number
  auto?: boolean
  created_at: string
  updated_at: string
  reviewed_at?: string
  merged_into?: string
  provenance?: MemoryCandidateProvenance
  similar?: MemoryCandidateHint[]
  conflicts?: MemoryCandidateHint[]
}

export type MemoryCandidateListResponse = {
  count: number
  items: MemoryCandidate[]
}

export type MemoryCandidateReviewResponse = {
  candidate: MemoryCandidate
}

export type SyspromptScope = 'workspace' | 'agent'

export type SyspromptPromptImpact = {
  section: string
  role: string
  max_chars: number
  chars: number
  estimated_tokens: number
  will_truncate: boolean
  truncated_chars?: number
}

export type SyspromptFile = {
  scope: SyspromptScope
  path: string
  title: string
  description: string
  exists: boolean
  editable: boolean
  size_bytes?: number
  updated_at?: string
  content?: string
  starter_content?: string
  prompt_targets?: string[]
  prompt_impact?: SyspromptPromptImpact
}

export type SyspromptPreview = {
  target: 'main_agent' | 'sub_agent'
  prompt: string
  static_tokens: number
  relevant_tokens: number
  relevant_memory_count: number
  total_tokens: number
}

export type MemorySearchMatch = {
  source: string
  date: string
  line: number
  snippet: string
}

export type MemorySearchResult = {
  query: string
  limit: number
  results: MemorySearchMatch[]
  message?: string
}

export type MemoryPrefetchItem = {
  source: string
  source_tag: string
  snippet: string
  tokens: number
}

export type MemoryPrefetchResult = {
  session_id: string
  query: string
  section: string
  items: MemoryPrefetchItem[]
  relevant_tokens: number
  relevant_memory_count: number
  relevant_budget_tokens: number
  budget_percent: number
  message?: string
  generated_at: string
}

export type CreateCronJobRequest = {
  name?: string
  prompt: string
  schedule?: string
  enabled?: boolean
  session_id?: string
  session_target?: string
  wake_mode?: string
  delivery_mode?: string
  payload?: Record<string, unknown>
  delete_after_run?: boolean
}

export type UpdateCronJobRequest = {
  name?: string
  prompt?: string
  schedule?: string
  enabled?: boolean
  session_id?: string
  session_target?: string
  wake_mode?: string
  delivery_mode?: string
  payload?: Record<string, unknown>
  delete_after_run?: boolean
}

export type CronRunResult = {
  job_id: string
  job_name: string
  response?: string
  error?: string
}

// --- Hub / Extensions ---

export type HubRegistryEntry = {
  name: string
  description: string
  version: string
  author: string
  tags: string[]
  path: string
  user_invocable?: boolean
  requires_plugin?: string
  files?: string[] | { path: string; sha256: string }[]
  manifest?: string
}

export type HubRegistry = {
  version: number
  skills: HubRegistryEntry[]
  plugins: HubRegistryEntry[]
  mcp_servers: HubRegistryEntry[]
}

export type SkillSandboxCheck = {
  name: string
  command?: string
  status: 'passed' | 'failed'
  output?: string
  error?: string
  duration_ms?: number
}

export type SkillSandboxReport = {
  skill_name: string
  workspace_dir?: string
  skill_dir?: string
  passed: boolean
  checks: SkillSandboxCheck[]
}

export type HubInstallResponse = {
  ok: string
  type: string
  name: string
  requires_plugin?: string
  sandbox_report?: SkillSandboxReport
}

export type HubInstalledItem = {
  name: string
  version: string
  source: string
  dir: string
  manifest?: string
}

export type HubInstalled = {
  skills: HubInstalledItem[]
  plugins: HubInstalledItem[]
  mcps: HubInstalledItem[]
}

export type SkillDef = {
  name: string
  description: string
  slash?: string
  aliases?: string[]
  source?: string
  user_invocable?: boolean
  available?: boolean
}

export type PluginDef = {
  id: string
  name: string
  description?: string
  version?: string
  available?: boolean
}

export type MCPServerStatus = {
  name: string
  transport?: string
  status?: string
  source?: string
  tools_count?: number
}

export type SkillCreatorFile = {
  path: string
  content: string
}

export type SkillCreatorDraftRequest = {
  name: string
  description: string
  category?: string
  language: 'python' | 'typescript' | 'shell'
  layout: 'single_file' | 'directory'
  use_case: string
  recommended_tools: string[]
}

export type SkillCreatorDraftResponse = SkillCreatorDraftRequest & {
  files: SkillCreatorFile[]
  warnings?: string[]
}

export type SkillCreatorSaveResponse = {
  saved: boolean
  path: string
  files: string[]
}

export type SkillCreatorToolTrail = {
  tool: string
  command: string
  cwd: string
  status: string
  exit_code: number
  duration_ms: number
}

export type SkillCreatorTestResponse = {
  success: boolean
  exit_code: number
  stdout: string
  stderr: string
  sandbox_path: string
  session_kind: string
  hidden: boolean
  duration_ms: number
  tool_trail: SkillCreatorToolTrail[]
}

export type SkillCreatorSubmitResponse = {
  submitted: boolean
  ready: boolean
  message: string
  commands?: string[]
}

export type MCPServerCreatorToolSpec = {
  name: string
  description: string
  input_schema?: Record<string, unknown>
  output_schema?: Record<string, unknown>
}

export type MCPServerCreatorFile = {
  path: string
  content: string
}

export type MCPServerCreatorDraftRequest = {
  name: string
  description: string
  language: 'python' | 'node'
  use_case: string
  tools?: MCPServerCreatorToolSpec[]
}

export type MCPServerCreatorDraftResponse = {
  name: string
  description: string
  language: 'python' | 'node'
  use_case: string
  tools: MCPServerCreatorToolSpec[]
  files: MCPServerCreatorFile[]
  warnings?: string[]
}

export type MCPServerCreatorSaveResponse = {
  saved: boolean
  path: string
  files: string[]
}

export type MCPServerCreatorToolTrail = {
  tool: string
  command: string
  cwd: string
  status: string
  exit_code: number
  duration_ms: number
}

export type MCPServerCreatorTestResponse = {
  success: boolean
  exit_code: number
  stdout?: string
  stderr?: string
  tools: string[]
  call_result?: string
  protocol_steps: string[]
  sandbox_path: string
  session_kind: string
  hidden: boolean
  duration_ms: number
  tool_trail: MCPServerCreatorToolTrail[]
}

export type MCPServerCreatorSubmitResponse = {
  submitted: boolean
  ready: boolean
  message: string
  commands?: string[]
}

export type ConfigFile = {
  path: string
  content: string
}

export type ConfigFieldMeta = {
  key: string
  path: string
  section: string
  type: string
  label: string
  description: string
  impact?: string[]
  default_value?: unknown
  requires_restart?: boolean
  sensitive?: boolean
  options?: string[]
}

export type ConfigSchema = {
  path: string
  updated_at?: string
  fields: ConfigFieldMeta[]
  values: Record<string, unknown>
}

export type ProviderModelsInfo = {
  provider: string
  current_model: string
  source: string
  stale: boolean
  fetched_at?: string
  expires_at?: string
  models: string[]
  warning?: string
}

export type PlanStatus =
  | 'drafting'
  | 'proposed'
  | 'executing'
  | 'paused'
  | 'completed'
  | 'aborted'

export type SessionPlan = {
  goal: string
  constraints?: string
  created_at: string
  status?: PlanStatus
  updated_at?: string
}

export type ContractStatus = 'draft' | 'approved'

export type TaskContract = {
  goal?: string
  scope?: string
  done_criteria?: string[]
  verification_commands?: string[]
  artifacts?: string[]
  status?: ContractStatus
  created_at?: string
  updated_at?: string
}

export type TaskEvidenceType =
  | 'test_result'
  | 'image'
  | 'log_excerpt'
  | 'pr_link'
  | 'release_tag'
  | 'command_output_summary'

export type TaskEvidence = {
  id: string
  type: TaskEvidenceType | string
  title?: string
  summary?: string
  url?: string
  command?: string
  path?: string
  status?: string
  created_at?: string
}

export type SessionTask = {
  id: string
  title: string
  status: string
  description?: string
  evidence?: TaskEvidence[]
}

export type SessionTasks = {
  plan?: SessionPlan
  contract?: TaskContract
  tasks: SessionTask[]
}

export type GitRemote = {
  name: string
  fetch_url?: string
  push_url?: string
}

export type GitStatusFile = {
  path: string
  old_path?: string
  index?: string
  worktree?: string
  status: string
  staged: boolean
  unstaged: boolean
  untracked?: boolean
}

export type GitStatus = {
  is_git: boolean
  root: string
  branch?: string
  head?: string
  upstream?: string
  remotes?: GitRemote[]
  files?: GitStatusFile[]
}

export type GitDiff = {
  is_git: boolean
  root: string
  path?: string
  staged: boolean
  patch: string
}

export type GitCommit = {
  hash: string
  short_hash: string
  author?: string
  date?: string
  subject: string
}

export type GitLogResponse = {
  is_git: boolean
  root: string
  commits: GitCommit[]
}

export type GitBranch = {
  name: string
  current?: boolean
  upstream?: string
  remote?: boolean
  head?: string
}

export type GitBranchesResponse = {
  is_git: boolean
  root: string
  branches: GitBranch[]
}

export type GlobalPlanItem = {
  session: Session
  plan: SessionPlan
  contract?: TaskContract
  tasks: SessionTask[]
  summary: Record<string, number>
  updated_at: string
}

export type GlobalPlansResponse = {
  items: GlobalPlanItem[]
  count: number
}

export type PlanArchiveItem = {
  id: string
  session_id?: string
  goal: string
  archived_at: string
  created_at?: string
  summary: string
}

export type PlanArchiveResponse = {
  items: PlanArchiveItem[]
  count: number
}

export type SessionWorkDirs = {
  work_dirs: string[]
  current_dir: string
}
