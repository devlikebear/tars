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
  return readJSON<T>(response)
}

// APITarget is a TARS server other than the page's own: the mobile app
// (docs/decisions/mobile-remote.md) talks to each paired desktop at its
// base URL with that desktop's device token, never with a cookie.
export type APITarget = { baseURL: string; token: string }

export async function requestJSONAt<T>(target: APITarget, path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  return readJSON<T>(await fetchAt(target, path, { ...init, headers }))
}

// fetchAt sends one request to a desktop: its address, its bearer token,
// never a cookie. Streams (the turn feed) use it directly; JSON calls go
// through requestJSONAt.
export function fetchAt(target: APITarget, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  if (target.token) headers.set('Authorization', `Bearer ${target.token}`)
  return fetch(`${target.baseURL.replace(/\/+$/, '')}${path}`, {
    ...init,
    credentials: 'omit',
    headers,
  })
}

async function readJSON<T>(response: Response): Promise<T> {
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
