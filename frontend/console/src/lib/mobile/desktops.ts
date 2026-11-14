// The mobile app's list of desktops (docs/decisions/mobile-remote.md §2).
// Each desktop runs its own TARS server; the phone holds one entry per
// desktop with the device token that desktop gave it at pairing. Nothing is
// shared between desktops.
import type { APITarget } from '../api/client.ts'
import { normalizeBaseURL, type PairingOptions } from '../devicePairing.ts'

export type PairedDesktop = {
  // The base URL: one entry per desktop, so pairing again replaces it.
  id: string
  name: string
  baseURL: string
  token: string
  // The id the desktop gave this phone (its row in the desk's device list).
  deviceID: string
  pairedAt: string
}

// What the phone knows about a desktop right now.
//   online      — it answers and the token works
//   unreachable — no answer (asleep, off the tailnet)
//   unpaired    — it answers but refuses the token (revoked at the desk)
//   setup       — it answers but has no LLM configured yet
export type DesktopState = 'checking' | 'online' | 'unreachable' | 'unpaired' | 'setup'

export type DesktopStatus = { state: DesktopState; version?: string }

export function targetOf(desktop: PairedDesktop): APITarget {
  return { baseURL: desktop.baseURL, token: desktop.token }
}

// desktopName is a desktop's first host label: "mac-mini" for
// https://mac-mini.tail1.ts.net, the same name the Telegram alert uses. An
// address (a development build pairing over loopback) is kept whole.
export function desktopName(baseURL: string): string {
  try {
    const url = new URL(baseURL)
    const host = url.hostname
    if (/^\d+(\.\d+){3}$/.test(host) || host.startsWith('[') || host === 'localhost') return url.host
    return host.split('.')[0] || host
  } catch {
    return baseURL
  }
}

// addressToBaseURL reads what someone typed as a desktop's address: the
// Tailscale name alone ("mac-mini.tail1.ts.net") means https.
export function addressToBaseURL(text: string, opts: PairingOptions = {}): string {
  const trimmed = text.trim()
  if (!trimmed) return ''
  return normalizeBaseURL(/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`, opts)
}

// withDesktop adds a desktop, replacing an earlier pairing with the same
// base URL in place so the list keeps its order.
export function withDesktop(list: PairedDesktop[], desktop: PairedDesktop): PairedDesktop[] {
  const at = list.findIndex((d) => d.id === desktop.id)
  if (at < 0) return [...list, desktop]
  return list.map((d, i) => (i === at ? desktop : d))
}

export function withoutDesktop(list: PairedDesktop[], id: string): PairedDesktop[] {
  return list.filter((d) => d.id !== id)
}

// parseDesktops reads the stored list, dropping any entry that is not whole.
export function parseDesktops(raw: string | null | undefined): PairedDesktop[] {
  if (!raw) return []
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(value)) return []
  const out: PairedDesktop[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') continue
    const d = item as Record<string, unknown>
    const fields = ['id', 'name', 'baseURL', 'token', 'deviceID', 'pairedAt'] as const
    if (!fields.every((f) => typeof d[f] === 'string' && d[f] !== '')) continue
    out.push({
      id: d.id as string,
      name: d.name as string,
      baseURL: d.baseURL as string,
      token: d.token as string,
      deviceID: d.deviceID as string,
      pairedAt: d.pairedAt as string,
    })
  }
  return out
}

export type HealthAnswer = { ok?: boolean; needs_setup?: boolean; version?: string }

// statusFrom turns the two probes into one state. health is null when the
// desktop did not answer; tokenStatus is the HTTP status of an authenticated
// read (0 when it did not answer).
export function statusFrom(health: HealthAnswer | null, tokenStatus: number): DesktopStatus {
  if (!health) return { state: 'unreachable' }
  const version = health.version || undefined
  if (tokenStatus === 401 || tokenStatus === 403) return { state: 'unpaired', version }
  if (health.needs_setup) return { state: 'setup', version }
  if (tokenStatus >= 200 && tokenStatus < 300) return { state: 'online', version }
  return { state: 'unreachable', version }
}
