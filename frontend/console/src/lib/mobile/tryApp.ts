// "Try it" on the phone (vibe redesign unit 7, docs/decisions/vibe-phone-
// preview.md): a finished project's app, opened on the paired iPhone
// through the desk's preview relay. The order is fixed by the ticket: it
// lives 60 seconds and opens once, so it is asked for only once the app
// answers, and a new one is asked for on every press.
//
//   1. ask for a ticket. That also starts the app, and a refusal says at
//      once when the phone cannot reach it at all (phone access off, an
//      older desk, a project not ready);
//   2. if the app was still starting, let that ticket go, read the app's
//      state until it runs (or fails), and ask for a fresh ticket;
//   3. open the ticket's address outside the app.
import { APIRequestError } from '../api/client.ts'
import { easyErrorCode, type EasyAPI, type EasyPreviewTicket } from '../api/easy.ts'
import { previewPollMs, type EasyPreview } from '../easy.ts'
import { demoBaseURL } from './demoDesk.ts'

// Why the app could not be opened, each said in its own words on the
// screen (phone.ts `tryIt.errors`).
export type TryAppFailure =
  // The project is not finished yet, or it is gone from the desk.
  | 'not_ready'
  // The app did not start on the desk.
  | 'could_not_start'
  // The desk cannot show it to a phone right now: remote access is off,
  // Tailscale does not answer, or the relay is not running or not served.
  | 'phone_off'
  // A desk from before the ticket route answers 403: the app stays on it.
  | 'too_old'
  // The desk did not answer.
  | 'no_answer'
  // Anything else the desk refused.
  | 'failed'

export type TryAppOutcome =
  | { kind: 'open'; url: string }
  // The demo desk has no app to show; the screen says so in place.
  | { kind: 'demo' }
  | { kind: 'failed'; reason: TryAppFailure }

// The server gives a starting app 90 seconds before it calls it failed;
// past this the phone stops waiting and says it did not start.
export const tryAppWaitMs = 120_000

const phoneOffCodes = new Set(['remote_access_off', 'relay_not_served', 'tailscale_unavailable', 'relay_down'])

// tryAppFailure reads a refused request as one of the failures above.
export function tryAppFailure(err: unknown): TryAppFailure {
  if (!(err instanceof APIRequestError)) return 'no_answer'
  const code = easyErrorCode(err)
  if (phoneOffCodes.has(code)) return 'phone_off'
  if (code === 'preview_failed') return 'could_not_start'
  if (code === 'not_ready' || code === 'not_found' || err.status === 404) return 'not_ready'
  if (err.status === 403) return 'too_old'
  if (err.status === 503) return 'phone_off'
  return 'failed'
}

export type TryAppOptions = {
  sleep?: (ms: number) => Promise<void>
  now?: () => number
  // Stops the wait when the screen goes away; the outcome is then ignored.
  cancelled?: () => boolean
}

const realSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

// tryApp runs the sequence and says what to do. It never opens anything
// itself: the screen opens `url` the moment it gets it.
export async function tryApp(api: EasyAPI, projectId: string, options: TryAppOptions = {}): Promise<TryAppOutcome> {
  const sleep = options.sleep ?? realSleep
  const now = options.now ?? Date.now
  const fail = (reason: TryAppFailure): TryAppOutcome => ({ kind: 'failed', reason })
  let ticket: EasyPreviewTicket
  try {
    ticket = await api.previewTicket(projectId)
  } catch (err) {
    return fail(tryAppFailure(err))
  }
  let preview: EasyPreview = ticket.preview ?? { state: 'running' }
  if (preview.state !== 'running') {
    const until = now() + tryAppWaitMs
    let restarted = false
    while (preview.state !== 'running') {
      if (preview.state === 'failed') return fail('could_not_start')
      // Stopped while we waited (the desk's idle sweep): start it once more.
      if (preview.state === 'stopped') {
        if (restarted) return fail('could_not_start')
        restarted = true
        try {
          preview = await api.startPreview(projectId)
        } catch (err) {
          return fail(tryAppFailure(err))
        }
        continue
      }
      if (now() >= until) return fail('could_not_start')
      await sleep(previewPollMs(preview) ?? 1000)
      if (options.cancelled?.()) return fail('failed')
      try {
        preview = await api.preview(projectId)
      } catch (err) {
        return fail(tryAppFailure(err))
      }
    }
    // The first ticket may be near its end by now: ask for a fresh one.
    try {
      ticket = await api.previewTicket(projectId)
    } catch (err) {
      return fail(tryAppFailure(err))
    }
  }
  if (isDemoURL(ticket.url)) return { kind: 'demo' }
  return { kind: 'open', url: ticket.url }
}

function isDemoURL(url: string): boolean {
  try {
    return new URL(url).origin === demoBaseURL
  } catch {
    return false
  }
}
