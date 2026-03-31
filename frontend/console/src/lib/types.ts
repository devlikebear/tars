export type HeartbeatStatus = {
  configured: boolean
  interval?: string
  active_hours?: string
  timezone?: string
  chat_busy?: boolean
  last_run_at?: string
  last_skipped?: boolean
  last_skip_reason?: string
  last_logged?: boolean
  last_acknowledged?: boolean
  last_response?: string
  last_error?: string
}

export type HeartbeatRunResult = {
  response?: string
  skipped?: boolean
  skip_reason?: string
  acknowledged?: boolean
  logged?: boolean
}

export type Project = {
  id: string
  name: string
  type: string
  status: string
  git_repo?: string
  created_at?: string
  updated_at?: string
  objective?: string
  path?: string
  body?: string
  session_id?: string
}

export type ProjectSessionInfo = {
  project_id: string
  session_id: string
  messages: number
  tokens: number
}

export type ProjectActivity = {
  id: string
  project_id: string
  task_id?: string
  source: string
  agent?: string
  kind: string
  status?: string
  message?: string
  timestamp: string
  meta?: Record<string, string>
}

export type CronJob = {
  id: string
  name: string
  prompt: string
  schedule: string
  enabled: boolean
  delete_after_run?: boolean
  session_target?: string
  project_id?: string
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
  project_id?: string
  session_id?: string
  open_path?: string
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
  project_id?: string
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
}

export type ChatEvent = {
  type: string
  text?: string
  error?: string
  session_id?: string
  message?: string
  phase?: string
  tool_name?: string
  tool_call_id?: string
  tool_args_preview?: string
  tool_result_preview?: string
  skill_name?: string
  skill_reason?: string
}

export type ChatAttachment = {
  name: string
  mime_type: string
  data: string // base64
}

export type ChatRequest = {
  message: string
  session_id?: string
  project_id?: string
  attachments?: ChatAttachment[]
}

export type CreateProjectRequest = {
  name: string
  type?: string
  git_repo?: string
  objective?: string
  instructions?: string
}

export type UpdateProjectRequest = {
  name?: string
  type?: string
  status?: string
  git_repo?: string
  objective?: string
  instructions?: string
}

export type CreateCronJobRequest = {
  name?: string
  prompt: string
  schedule?: string
  enabled?: boolean
  session_target?: string
  project_id?: string
}

export type UpdateCronJobRequest = {
  name?: string
  prompt?: string
  schedule?: string
  enabled?: boolean
  session_target?: string
  project_id?: string
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

export type ConfigFile = {
  path: string
  content: string
}

export type ConfigFieldMeta = {
  key: string
  section: string
  type: string
  label: string
  description: string
  sensitive?: boolean
  options?: string[]
}

export type ConfigSchema = {
  path: string
  fields: ConfigFieldMeta[]
  values: Record<string, unknown>
}
