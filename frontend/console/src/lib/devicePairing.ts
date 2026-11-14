// Pairing a phone with this desktop (docs/decisions/mobile-remote.md §4).
//
// The desk shows a QR code; the phone scans it and trades the one-time code
// at the address in it for a focus device token. Both sides read the same
// link through this module, so the format lives in one place:
//
//   tars-mobile://pair?url=<https base URL>&code=<pairing code>
//
// The scheme is the app's own, so scanning with the phone's camera opens the
// app. The base URL must be https: a phone reaches a desktop only through
// its Tailscale serve address (§3), never over plain http.
import qrcode from 'qrcode-generator'

export const PAIRING_SCHEME = 'tars-mobile:'

export type PairingTarget = { baseURL: string; code: string }

// normalizeBaseURL keeps an https origin (with its port) and drops any path,
// query or trailing slash; anything else is "".
export function normalizeBaseURL(raw: string): string {
  let url: URL
  try {
    url = new URL(raw.trim())
  } catch {
    return ''
  }
  if (url.protocol !== 'https:' || !url.hostname || url.username || url.password) return ''
  return url.origin
}

export function pairingLink(baseURL: string, code: string): string {
  const base = normalizeBaseURL(baseURL)
  const trimmed = code.trim()
  if (!base || !trimmed) return ''
  const params = new URLSearchParams({ url: base, code: trimmed })
  return `${PAIRING_SCHEME}//pair?${params.toString()}`
}

// parsePairingLink reads a scanned link; null when it is not one.
export function parsePairingLink(text: string): PairingTarget | null {
  let url: URL
  try {
    url = new URL(text.trim())
  } catch {
    return null
  }
  if (url.protocol !== PAIRING_SCHEME || url.hostname !== 'pair') return null
  const baseURL = normalizeBaseURL(url.searchParams.get('url') ?? '')
  const code = (url.searchParams.get('code') ?? '').trim()
  if (!baseURL || !code) return null
  return { baseURL, code }
}

// pairingBaseURL is the address to put in the code: the Tailscale serve URL,
// only while serve is on and forwards to TARS (the same rule as the server's
// remoteaccess.Status.URL). Any other address would lead the phone nowhere.
export function pairingBaseURL(remote: { url?: string; serving: boolean; ownedByTARS: boolean }): string {
  if (!remote.serving || !remote.ownedByTARS) return ''
  return normalizeBaseURL(remote.url ?? '')
}

export type QRPicture = { size: number; path: string }

// qrPicture draws text as one SVG path of dark modules (one unit each), so
// the card renders it as plain markup instead of injecting the library's
// HTML. size includes a 4-module quiet zone on every side.
export function qrPicture(text: string): QRPicture {
  const qr = qrcode(0, 'M')
  qr.addData(text)
  qr.make()
  const count = qr.getModuleCount()
  const quiet = 4
  const parts: string[] = []
  for (let row = 0; row < count; row++) {
    for (let col = 0; col < count; col++) {
      if (qr.isDark(row, col)) parts.push(`M${col + quiet} ${row + quiet}h1v1h-1z`)
    }
  }
  return { size: count + quiet * 2, path: parts.join('') }
}

// secondsLeft is how long a code still works, never below zero.
export function secondsLeft(expiresAt: string, now: number): number {
  const at = Date.parse(expiresAt)
  if (Number.isNaN(at)) return 0
  return Math.max(0, Math.floor((at - now) / 1000))
}

export function formatCountdown(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

// newlyPaired is the first active device that was not in before: the phone
// that just traded the code shown on screen, so the card can close the code.
export function newlyPaired<T extends { id: string; active: boolean }>(before: string[], devices: T[]): T | null {
  return devices.find((d) => d.active && !before.includes(d.id)) ?? null
}

// withFolder adds a folder to the list a phone may start pipelines in,
// trimmed and without duplicates; withoutFolder takes one out.
export function withFolder(folders: string[], path: string): string[] {
  const next = path.trim()
  if (!next || folders.includes(next)) return folders
  return [...folders, next]
}

export function withoutFolder(folders: string[], path: string): string[] {
  return folders.filter((f) => f !== path)
}
