import type { APIErrorPayload, ChatEvent, ChatRequest, Project, ProjectAutopilotRun } from './types'

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
