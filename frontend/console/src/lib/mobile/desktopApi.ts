// Calls the mobile app makes to a desktop before it has a screen open there:
// trade a pairing code for a token, and check that the desktop answers and
// still accepts the token. Each call names its desktop; nothing here uses
// the page's own origin.
import { APIRequestError, requestJSONAt, type APITarget } from '../api/client.ts'
import type { PairingTarget } from '../devicePairing.ts'
import { statusFrom, type DesktopStatus, type HealthAnswer } from './desktops.ts'

export type DevicePairAnswer = { device_id: string; name: string; role: string; token: string }

export type DesktopAPI = {
  pair(target: PairingTarget, phoneName: string): Promise<DevicePairAnswer>
  status(target: APITarget): Promise<DesktopStatus>
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
