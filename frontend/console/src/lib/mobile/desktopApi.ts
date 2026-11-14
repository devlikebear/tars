// Calls the mobile app makes to a desktop before it has a screen open there:
// trade a pairing code for a token, and check that the desktop answers and
// still accepts the token. Each call names its desktop; nothing here uses
// the page's own origin.
import { APIRequestError, requestJSONAt, type APITarget } from '../api/client.ts'
import type { PairingTarget } from '../devicePairing.ts'
import type { ActivitySnapshot } from '../sessionBoard.ts'
import type { FocusListItem } from '../types.ts'
import { statusFrom, type DesktopStatus, type HealthAnswer } from './desktops.ts'

export type DevicePairAnswer = { device_id: string; name: string; role: string; token: string }

export type DesktopAPI = {
  pair(target: PairingTarget, phoneName: string): Promise<DevicePairAnswer>
  status(target: APITarget): Promise<DesktopStatus>
}

// The calls behind the inbox: what each desktop's pipelines wait on, and
// answering a tool call that waits. A phone answers only allow_once or deny
// to a question; the desk refuses answers that outlive it.
export type InboxAPI = {
  pipelines(target: APITarget): Promise<FocusListItem[]>
  activity(target: APITarget): Promise<ActivitySnapshot>
  reviewApproval(target: APITarget, approvalID: string, action: 'approve' | 'reject'): Promise<void>
  answerPermission(target: APITarget, requestID: string, sessionID: string, decision: 'allow_once' | 'deny'): Promise<void>
}

// probeTimeoutMs bounds a probe: a desktop that sleeps never answers, and the
// list should say so instead of spinning.
const probeTimeoutMs = 6000

export const desktopAPI: DesktopAPI = {
  pair(target, phoneName) {
    return requestJSONAt<DevicePairAnswer>({ baseURL: target.baseURL, token: '' }, '/v1/auth/device-pair', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code: target.code, name: phoneName }),
    })
  },

  async status(target) {
    const signal = AbortSignal.timeout(probeTimeoutMs)
    let health: HealthAnswer | null = null
    try {
      health = await requestJSONAt<HealthAnswer>({ baseURL: target.baseURL, token: '' }, '/v1/healthz', { signal })
    } catch {
      return statusFrom(null, 0)
    }
    let tokenStatus = 0
    try {
      await requestJSONAt(target, '/v1/auth/whoami', { signal })
      tokenStatus = 200
    } catch (err) {
      tokenStatus = err instanceof APIRequestError ? err.status : 0
    }
    return statusFrom(health, tokenStatus)
  },
}

// inboxTimeoutMs bounds an inbox read the same way: one slow desktop shows
// as not answering instead of holding the whole inbox.
const inboxTimeoutMs = 8000

export const inboxAPI: InboxAPI = {
  pipelines(target) {
    return requestJSONAt<FocusListItem[]>(target, '/v1/focus/pipelines', { signal: AbortSignal.timeout(inboxTimeoutMs) })
  },

  activity(target) {
    return requestJSONAt<ActivitySnapshot>(target, '/v1/chat/activity', { signal: AbortSignal.timeout(inboxTimeoutMs) })
  },

  async reviewApproval(target, approvalID, action) {
    await requestJSONAt(target, `/v1/ops/approvals/${encodeURIComponent(approvalID)}/${action}`, { method: 'POST' })
  },

  async answerPermission(target, requestID, sessionID, decision) {
    await requestJSONAt(target, `/v1/chat/permissions/${encodeURIComponent(requestID)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionID, decision }),
    })
  },
}
