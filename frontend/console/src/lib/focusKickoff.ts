// A focus task's session doesn't exist yet while the developer is still
// pasting images into the goal field (FocusNewTask), so there is nowhere on
// the server to put them until POST /v1/focus/pipelines returns a session
// id. This in-memory handoff bridges that gap: FocusNewTask stashes the
// converted attachments under the new session id, and the focus store's
// kickoff() takes them (once) when it sends the goal as the first turn.
//
// Memory-only and intentionally so: a reload before the first turn goes out
// loses the images, same as any other in-flight, not-yet-persisted input —
// the goal text itself still goes through (it's on the pipeline already).
import type { ChatAttachment } from './types.ts'

const pending = new Map<string, ChatAttachment[]>()

export function stashKickoffAttachments(sessionId: string, attachments: ChatAttachment[]): void {
  if (!sessionId || attachments.length === 0) return
  pending.set(sessionId, attachments)
}

// takeKickoffAttachments returns and forgets the session's stashed
// attachments; a second call (or none stashed) returns undefined.
export function takeKickoffAttachments(sessionId: string): ChatAttachment[] | undefined {
  const attachments = pending.get(sessionId)
  pending.delete(sessionId)
  return attachments
}
