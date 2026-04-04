import type {
  APIErrorPayload,
  Approval,
  ChatEvent,
  ChatRequest,
  CleanupApplyResult,
  CleanupPlan,
  ConfigFile,
  ConfigSchema,
  HubInstalled,
  HubRegistry,
  MCPServerStatus,
  MemoryAsset,
  MemoryFile,
  MemorySearchResult,
  SyspromptFile,
  PluginDef,
  SkillDef,
  CreateCronJobRequest,
  CreateProjectRequest,
  CronJob,
  CronRunRecord,
  CronRunResult,
  EventsHistoryInfo,
  NotificationMessage,
  OpsStatus,
  Project,
  ProjectActivity,
  ProjectSessionInfo,
  ProjectBoard,
  ProjectState,
  BoardTask,
  HeartbeatStatus,
  HeartbeatRunResult,
  KnowledgeGraph,
  KnowledgeNote,
  Session,
  SessionMessage,
  UpdateCronJobRequest,
  SessionTasks,
  SessionWorkDirs,
  UpdateProjectRequest,
} from './types'

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

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

// --- Heartbeat ---

export async function getHeartbeatStatus(): Promise<HeartbeatStatus> {
  return requestJSON<HeartbeatStatus>('/v1/heartbeat/status')
}

export async function runHeartbeatOnce(): Promise<HeartbeatRunResult> {
  return requestJSON<HeartbeatRunResult>('/v1/heartbeat/run-once', { method: 'POST' })
}

export async function getHeartbeatConfig(): Promise<{ content: string }> {
  return requestJSON<{ content: string }>('/v1/heartbeat/config')
}

export async function saveHeartbeatConfig(content: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/heartbeat/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
}

export async function getHeartbeatLog(): Promise<{ date: string; content: string }> {
  return requestJSON<{ date: string; content: string }>('/v1/heartbeat/log')
}

// --- Projects ---

export async function listProjects(): Promise<Project[]> {
  return requestJSON<Project[]>('/v1/projects')
}

export async function getProject(projectId: string): Promise<Project> {
  return requestJSON<Project>(`/v1/projects/${encodeURIComponent(projectId)}`)
}

export async function getProjectSession(projectId: string): Promise<ProjectSessionInfo> {
  return requestJSON<ProjectSessionInfo>(
    `/v1/projects/${encodeURIComponent(projectId)}/session`,
  )
}

export async function clearProjectSession(projectId: string): Promise<{ cleared: boolean }> {
  return requestJSON<{ cleared: boolean }>(
    `/v1/projects/${encodeURIComponent(projectId)}/session/clear`,
    { method: 'POST' },
  )
}

export async function compactProjectSession(projectId: string): Promise<{ compacted: boolean; original_count: number; final_count: number }> {
  return requestJSON<{ compacted: boolean; original_count: number; final_count: number }>(
    `/v1/projects/${encodeURIComponent(projectId)}/session/compact`,
    { method: 'POST' },
  )
}

export async function listProjectActivity(projectId: string, limit = 20): Promise<ProjectActivity[]> {
  const payload = await requestJSON<{ count: number; items: ProjectActivity[] }>(
    `/v1/projects/${encodeURIComponent(projectId)}/activity?limit=${limit}`,
  )
  return payload.items ?? []
}

export async function getProjectBoard(projectId: string): Promise<ProjectBoard> {
  return requestJSON<ProjectBoard>(`/v1/projects/${encodeURIComponent(projectId)}/board`)
}

export async function updateProjectBoard(projectId: string, tasks: BoardTask[]): Promise<ProjectBoard> {
  return requestJSON<ProjectBoard>(`/v1/projects/${encodeURIComponent(projectId)}/board`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tasks }),
  })
}

export async function getProjectState(projectId: string): Promise<ProjectState> {
  return requestJSON<ProjectState>(`/v1/projects/${encodeURIComponent(projectId)}/state`)
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

export async function listSessions(includeHidden = false): Promise<Session[]> {
  const params = includeHidden ? '?hidden=1' : ''
  return requestJSON<Session[]>(`/v1/admin/sessions${params}`)
}

export async function getSession(sessionId: string): Promise<Session> {
  return requestJSON<Session>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}`)
}

export async function getSessionHistory(sessionId: string): Promise<SessionMessage[]> {
  return requestJSON<SessionMessage[]>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/history`)
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

export async function compactSession(sessionId: string): Promise<{ compacted: boolean }> {
  return requestJSON<{ compacted: boolean }>(
    `/v1/admin/sessions/${encodeURIComponent(sessionId)}/compact`,
    { method: 'POST' },
  )
}

export async function getSessionTasks(sessionId: string): Promise<SessionTasks> {
  return requestJSON<SessionTasks>(`/v1/admin/sessions/${encodeURIComponent(sessionId)}/tasks`)
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

// --- Knowledge Base ---

export async function listKnowledgeNotes(params: {
  query?: string
  kind?: string
  tag?: string
  project_id?: string
  limit?: number
} = {}): Promise<{ count: number; items: KnowledgeNote[] }> {
  const search = new URLSearchParams()
  if (params.query?.trim()) search.set('query', params.query.trim())
  if (params.kind?.trim()) search.set('kind', params.kind.trim())
  if (params.tag?.trim()) search.set('tag', params.tag.trim())
  if (params.project_id?.trim()) search.set('project_id', params.project_id.trim())
  if (params.limit && params.limit > 0) search.set('limit', String(params.limit))
  const qs = search.toString()
  return requestJSON<{ count: number; items: KnowledgeNote[] }>(`/v1/memory/kb/notes${qs ? `?${qs}` : ''}`)
}

export async function getKnowledgeNote(slug: string): Promise<KnowledgeNote> {
  return requestJSON<KnowledgeNote>(`/v1/memory/kb/notes/${encodeURIComponent(slug)}`)
}

export async function createKnowledgeNote(payload: Partial<KnowledgeNote>): Promise<KnowledgeNote> {
  return requestJSON<KnowledgeNote>('/v1/memory/kb/notes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function updateKnowledgeNote(slug: string, payload: Partial<KnowledgeNote>): Promise<KnowledgeNote> {
  return requestJSON<KnowledgeNote>(`/v1/memory/kb/notes/${encodeURIComponent(slug)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function deleteKnowledgeNote(slug: string): Promise<{ deleted: boolean; slug: string }> {
  return requestJSON<{ deleted: boolean; slug: string }>(`/v1/memory/kb/notes/${encodeURIComponent(slug)}`, {
    method: 'DELETE',
  })
}

export async function getKnowledgeGraph(): Promise<KnowledgeGraph> {
  return requestJSON<KnowledgeGraph>('/v1/memory/kb/graph')
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

export async function runMemorySearch(payload: {
  query: string
  limit?: number
  include_memory?: boolean
  include_daily?: boolean
  include_knowledge?: boolean
  include_sessions?: boolean
}): Promise<MemorySearchResult> {
  return requestJSON<MemorySearchResult>('/v1/memory/search', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
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

// --- Session Config ---

export type SessionToolConfig = {
  tools_enabled?: string[]
  tools_disabled?: string[]
  skills_enabled?: string[]
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

export type ChatToolInfo = {
  name: string
  description: string
  high_risk: boolean
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
  memory_count: number
  memory_tokens: number
  prompt_override: string
}

export async function getChatContext(sessionId: string): Promise<ChatContextInfo> {
  return requestJSON<ChatContextInfo>(`/v1/chat/context?session_id=${encodeURIComponent(sessionId)}`)
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

// --- Project CRUD ---

export async function createProject(data: CreateProjectRequest): Promise<Project> {
  return requestJSON<Project>('/v1/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function updateProject(projectId: string, data: UpdateProjectRequest): Promise<Project> {
  return requestJSON<Project>(`/v1/projects/${encodeURIComponent(projectId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function deleteProject(projectId: string): Promise<void> {
  await requestJSON<Record<string, never>>(`/v1/projects/${encodeURIComponent(projectId)}`, {
    method: 'DELETE',
  })
}

export async function deleteAllProjects(): Promise<{ deleted: number }> {
  return requestJSON<{ deleted: number }>('/v1/projects', { method: 'DELETE' })
}

export type ProjectFile = {
  name: string
  size: number
  system: boolean
}

export type ProjectFileContent = {
  name: string
  content: string
  size: number
}

export async function listProjectFiles(projectId: string): Promise<ProjectFile[]> {
  return requestJSON<ProjectFile[]>(`/v1/projects/${encodeURIComponent(projectId)}/files`)
}

export async function getProjectFileContent(projectId: string, filename: string): Promise<ProjectFileContent> {
  return requestJSON<ProjectFileContent>(`/v1/projects/${encodeURIComponent(projectId)}/files/${encodeURIComponent(filename)}`)
}

export async function activateProject(projectId: string): Promise<{ activated: boolean }> {
  return requestJSON<{ activated: boolean }>(
    `/v1/projects/${encodeURIComponent(projectId)}/activate`,
    { method: 'POST' },
  )
}

export async function deactivateProject(projectId: string): Promise<{ deactivated: boolean }> {
  return requestJSON<{ deactivated: boolean }>(
    `/v1/projects/${encodeURIComponent(projectId)}/deactivate`,
    { method: 'POST' },
  )
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

export async function resetWorkspace(): Promise<{ removed_dirs: number }> {
  return requestJSON<{ removed_dirs: number }>('/v1/admin/reset/workspace', { method: 'POST' })
}

export async function patchConfigValues(updates: Record<string, unknown>): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/admin/config/values', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ updates }),
  })
}

// --- Hub / Extensions ---

export async function getHubRegistry(): Promise<HubRegistry> {
  return requestJSON<HubRegistry>('/v1/hub/registry')
}

export async function getHubInstalled(): Promise<HubInstalled> {
  return requestJSON<HubInstalled>('/v1/hub/installed')
}

export async function hubInstall(type: string, name: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/hub/install', {
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

// --- Events ---

export function streamEvents(
  projectId: string | undefined,
  onEvent: (event: NotificationMessage) => void,
  onError?: (message: string) => void,
  onOpen?: () => void,
): () => void {
  const params = projectId ? `?project_id=${encodeURIComponent(projectId)}` : ''
  const stream = new EventSource(`/v1/events/stream${params}`)
  stream.onopen = () => {
    onOpen?.()
  }
  stream.onmessage = (message) => {
    if (!message.data) {
      return
    }
    try {
      const payload = JSON.parse(message.data) as NotificationMessage
      if (payload.type === 'keepalive') {
        return
      }
      onEvent(payload)
    } catch (error) {
      onError?.(error instanceof Error ? error.message : 'Failed to parse event stream payload')
    }
  }
  stream.onerror = () => {
    onError?.('Event stream disconnected')
  }
  return () => {
    stream.close()
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
  content: string
}

export async function listWorkspaceFiles(path = '.', root?: string): Promise<{ path: string; files: WorkspaceFileEntry[] }> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<{ path: string; files: WorkspaceFileEntry[] }>(
    `/v1/workspace/files?${params}`
  )
}

export async function readWorkspaceFile(path: string, root?: string): Promise<WorkspaceFileContent> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<WorkspaceFileContent>(
    `/v1/workspace/files?${params}`
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
