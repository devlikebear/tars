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
}

export type ChatRequest = {
  message: string
  session_id?: string
  project_id?: string
}
