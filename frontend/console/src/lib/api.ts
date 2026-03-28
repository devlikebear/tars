import type {
  APIErrorPayload,
  Approval,
  ChatEvent,
  ChatRequest,
  CronJob,
  CronRunRecord,
  EventsHistoryInfo,
  NotificationMessage,
  Project,
  ProjectActivity,
  ProjectAutopilotRun,
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

  return (await response.json()) as T
}

export async function listProjects(): Promise<Project[]> {
  return requestJSON<Project[]>('/v1/projects')
}

export async function getProject(projectId: string): Promise<Project> {
  return requestJSON<Project>(`/v1/projects/${encodeURIComponent(projectId)}`)
}

export async function getProjectAutopilot(projectId: string): Promise<ProjectAutopilotRun | null> {
  const response = await fetch(`/v1/projects/${encodeURIComponent(projectId)}/autopilot`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })

  if (response.status === 404) {
    return null
  }
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

  return (await response.json()) as ProjectAutopilotRun
}

export async function listProjectActivity(projectId: string, limit = 20): Promise<ProjectActivity[]> {
  const payload = await requestJSON<{ count: number; items: ProjectActivity[] }>(
    `/v1/projects/${encodeURIComponent(projectId)}/activity?limit=${limit}`,
  )
  return payload.items ?? []
}

export async function listCronJobs(): Promise<CronJob[]> {
  return requestJSON<CronJob[]>('/v1/cron/jobs')
}

export async function listCronRuns(jobId: string, limit = 5): Promise<CronRunRecord[]> {
  return requestJSON<CronRunRecord[]>(`/v1/cron/jobs/${encodeURIComponent(jobId)}/runs?limit=${limit}`)
}

export async function listApprovals(): Promise<Approval[]> {
  return requestJSON<Approval[]>('/v1/ops/approvals')
}

export async function reviewApproval(approvalId: string, action: 'approve' | 'reject'): Promise<void> {
  await requestJSON<{ ok: boolean }>(`/v1/ops/approvals/${encodeURIComponent(approvalId)}/${action}`, {
    method: 'POST',
  })
}

export async function getEventsHistory(limit = 30): Promise<EventsHistoryInfo> {
  return requestJSON<EventsHistoryInfo>(`/v1/events/history?limit=${limit}`)
}

export function streamEvents(
  projectId: string,
  onEvent: (event: NotificationMessage) => void,
  onError?: (message: string) => void,
): () => void {
  const stream = new EventSource(`/v1/events/stream?project_id=${encodeURIComponent(projectId)}`)
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
    onError?.('Project event stream disconnected')
  }
  return () => {
    stream.close()
  }
}

export async function streamChat(
  request: ChatRequest,
  onEvent: (event: ChatEvent) => void,
): Promise<void> {
  const response = await fetch('/v1/chat', {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
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
