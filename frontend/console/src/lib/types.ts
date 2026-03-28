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
}

export type ProjectAutopilotRun = {
  project_id: string
  run_id: string
  status: string
  message?: string
  iterations: number
  started_at?: string
  updated_at?: string
  finished_at?: string
  phase?: string
  phase_status?: string
  next_action?: string
  summary?: string
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
  skill_name?: string
  skill_reason?: string
}

export type ChatRequest = {
  message: string
  session_id?: string
  project_id?: string
}
