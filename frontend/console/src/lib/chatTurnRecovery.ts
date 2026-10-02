// A chat turn runs on in the server when the console's stream to it breaks
// (#971). A panel that only reacted to the stream's own `done` then never
// learned the turn had ended: the transcript, title and session cost stayed
// as they were before it, so a new session read "$0 · 0 tokens" after
// minutes of work. The cost is recorded when the provider call returns, so
// it can only be read after the turn ends.

import { APIRequestError } from './api/client.ts'

// streamDropped tells a broken stream (network error, proxy reset) from the
// user stopping the turn, which aborts the fetch on purpose.
export function streamDropped(err: unknown): boolean {
  return !(err instanceof DOMException && err.name === 'AbortError')
}

// sendFailure tells why a send ended early:
//   - stopped: the user aborted the fetch.
//   - refused: POST /v1/chat answered non-2xx, so no turn started (409 while
//     the session's claim is still held: a turn winding down after a cancel,
//     a focus-driver turn). Nothing runs on in the server to recover.
//   - dropped: the stream broke; the turn may still be running.
export type SendFailure = 'stopped' | 'refused' | 'dropped'

export function sendFailure(err: unknown): SendFailure {
  if (!streamDropped(err)) return 'stopped'
  if (err instanceof APIRequestError) return 'refused'
  return 'dropped'
}

export type ReattachOutcome = {
  // The server still had the turn running and the panel followed it again.
  attached: boolean
  // The reattached stream reached `done` or `cancelled`, which settle the
  // turn themselves.
  ended: boolean
}

export type DroppedTurnDeps = {
  // Rebuild the thread from the transcript: the turn's user message is
  // already there, and the reattached feed replays only the reply.
  reloadHistory: () => Promise<void>
  reattach: () => Promise<ReattachOutcome>
  // Re-read what a finished turn changes: sessions, health, cost.
  settle: () => Promise<void>
}

// recoverDroppedTurn follows a turn whose stream broke. The turn is settled
// exactly once: by its own end event if the panel sees one, here otherwise
// (it ended during the gap, or the stream broke again).
export async function recoverDroppedTurn(deps: DroppedTurnDeps): Promise<void> {
  try {
    await deps.reloadHistory()
  } catch {
    // The reattach and the settle below still bring the rest back.
  }
  let outcome: ReattachOutcome = { attached: false, ended: false }
  try {
    outcome = await deps.reattach()
  } catch {
    // Still unreachable: settle with whatever the server can tell us.
  }
  if (!outcome.ended) await deps.settle()
}

// dropVerificationPlaceholder: a focus pipeline's verification feed (P2)
// streams progress, not a reply, so the empty assistant message the panel
// opened when it attached goes once the feed ends; a real reply stays.
export function dropVerificationPlaceholder<M extends { id: string; text?: string }>(messages: M[], placeholderId: string, verificationFeed: boolean): M[] {
  if (!verificationFeed) return messages
  return messages.filter((m) => m.id !== placeholderId || !!m.text)
}
