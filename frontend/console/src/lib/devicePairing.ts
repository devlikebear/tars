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
import type { FocusAnswerCounts, FocusAnswerReport } from './types.ts'

export const PAIRING_SCHEME = 'tars-mobile:'

export type PairingTarget = { baseURL: string; code: string }

// PairingOptions.allowLoopbackHTTP lets a development build of the app pair
// with a server on this machine (the iOS Simulator reaches the Mac's
// loopback). Release builds never pass it.
export type PairingOptions = { allowLoopbackHTTP?: boolean }

const loopbackHosts = new Set(['127.0.0.1', 'localhost', '[::1]'])

// normalizeBaseURL keeps an https origin (with its port) and drops any path,
// query or trailing slash; anything else is "".
export function normalizeBaseURL(raw: string, opts: PairingOptions = {}): string {
  let url: URL
  try {
    url = new URL(raw.trim())
  } catch {
    return ''
  }
  if (!url.hostname || url.username || url.password) return ''
  const loopbackHTTP = opts.allowLoopbackHTTP && url.protocol === 'http:' && loopbackHosts.has(url.hostname)
  if (url.protocol !== 'https:' && !loopbackHTTP) return ''
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
export function parsePairingLink(text: string, opts: PairingOptions = {}): PairingTarget | null {
  let url: URL
  try {
    url = new URL(text.trim())
  } catch {
    return null
  }
  if (url.protocol !== PAIRING_SCHEME || url.hostname !== 'pair') return null
  const baseURL = normalizeBaseURL(url.searchParams.get('url') ?? '', opts)
  const code = (url.searchParams.get('code') ?? '').trim()
  if (!baseURL || !code) return null
  return { baseURL, code }
}

// A pipeline on a desktop, in the app's own words (§7):
//
//   tars-mobile://focus?url=<https base URL>&session=<pipeline session id>
//
// Telegram makes only http(s) links tappable, so an alert links to the
// console's focus page; on an iPhone that page offers this link, which opens
// the same pipeline in the app on the desktop paired at that address.
export type FocusLinkTarget = { baseURL: string; sessionID: string }

export function focusAppLink(baseURL: string, sessionID: string, opts: PairingOptions = {}): string {
  const base = normalizeBaseURL(baseURL, opts)
  const session = sessionID.trim()
  if (!base || !session) return ''
  const params = new URLSearchParams({ url: base, session })
  return `${PAIRING_SCHEME}//focus?${params.toString()}`
}

// parseFocusLink reads a link the app was opened with; null when it is not one.
export function parseFocusLink(text: string, opts: PairingOptions = {}): FocusLinkTarget | null {
  let url: URL
  try {
    url = new URL(text.trim())
  } catch {
    return null
  }
  if (url.protocol !== PAIRING_SCHEME || url.hostname !== 'focus') return null
  const baseURL = normalizeBaseURL(url.searchParams.get('url') ?? '', opts)
  const sessionID = (url.searchParams.get('session') ?? '').trim()
  if (!baseURL || !sessionID) return null
  return { baseURL, sessionID }
}

// The browser the console is open in, as far as the hand-off cares.
export type HandoffBrowser = { userAgent: string; platform?: string; maxTouchPoints?: number }

// onApplePhone says whether the console is open on an iPhone or iPad, where
// the app exists (iPadOS reports itself as a Mac with a touch screen).
export function onApplePhone(browser: HandoffBrowser): boolean {
  if (/\b(iPhone|iPad|iPod)\b/.test(browser.userAgent)) return true
  return browser.platform === 'MacIntel' && (browser.maxTouchPoints ?? 0) > 1
}

// pipelineSessionOf is the pipeline session a console path shows, "" for any
// other page (the focus home, the release train, the template editor).
export function pipelineSessionOf(pathname: string): string {
  const match = /^\/console\/focus\/([^/?#]+)\/?$/.exec(pathname)
  if (!match) return ''
  let session = ''
  try {
    session = decodeURIComponent(match[1]).trim()
  } catch {
    return ''
  }
  return session === 'release' || session === 'templates' ? '' : session
}

// appHandoffLink is the link the console offers to open this page in the
// app: only on an iPhone or iPad, only on one pipeline's page, and only at an
// https address (the only kind the app pairs with). "" means offer nothing.
export function appHandoffLink(origin: string, pathname: string, browser: HandoffBrowser): string {
  if (!onApplePhone(browser)) return ''
  const session = pipelineSessionOf(pathname)
  return session ? focusAppLink(origin, session) : ''
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

// answerShare reads the answer report into what the Phones card says: how
// many gate and card answers came from a phone, out of all answers that
// recorded where they came from. null when none did yet.
export function answerShare(report: FocusAnswerReport | null | undefined): { phone: number; total: number; pipelines: number } | null {
  if (!report) return null
  const sum = (c: FocusAnswerCounts) => c.phone + c.desk + c.goal
  const total = sum(report.gates) + sum(report.cards)
  if (total === 0) return null
  return { phone: report.gates.phone + report.cards.phone, total, pipelines: report.phone_pipelines }
}
