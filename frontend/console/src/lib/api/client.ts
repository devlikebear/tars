import type { APIErrorPayload } from '../types'

export class APIRequestError extends Error {
  status: number
  payload?: APIErrorPayload

  constructor(message: string, status: number, payload?: APIErrorPayload) {
    super(message)
    this.name = 'APIRequestError'
    this.status = status
    this.payload = payload
  }

  get sandboxReport() {
    return this.payload?.sandbox_report
  }
}

export async function requestJSON<T>(input: string, init?: RequestInit): Promise<T> {
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
    throw new APIRequestError(message, response.status, payload)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}
