// Session worktrees (#971): isolate a session in a worktree of its own and
// bring its changes back (apply), keep them on the branch, or drop them.
import { requestJSON } from './client.ts'
import type { SessionWorktreeAction, SessionWorktreeView } from '../types.ts'

function worktreeURL(sessionId: string): string {
  return `/v1/admin/sessions/${encodeURIComponent(sessionId)}/worktree`
}

export async function getSessionWorktree(sessionId: string): Promise<SessionWorktreeView> {
  return requestJSON<SessionWorktreeView>(worktreeURL(sessionId))
}

export async function sessionWorktreeAction(
  sessionId: string,
  action: SessionWorktreeAction,
): Promise<{ result: Record<string, unknown>; view: SessionWorktreeView }> {
  return requestJSON(worktreeURL(sessionId), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  })
}

export async function setSessionIsolation(sessionId: string, isolation: '' | 'off'): Promise<SessionWorktreeView> {
  return requestJSON<SessionWorktreeView>(worktreeURL(sessionId), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ isolation }),
  })
}
