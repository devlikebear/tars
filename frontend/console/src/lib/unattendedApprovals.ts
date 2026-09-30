// Tool calls that unattended turns (cron, Telegram, subagents) wait to run,
// queued in the ops approvals because the session's permission mode asks
// (#970). The chat shows the ones for the open session so a person can answer
// them there as well as on the Ops page. Pure helpers, tested under Node.
import type { Approval, ToolPermissionRequest } from './types.ts'

export type PendingToolApproval = {
  id: string
  requestedAt: string
  request: ToolPermissionRequest
}

// pendingToolApprovals picks the open tool_permission questions of one
// session, oldest first so they read in the order the run asked them.
export function pendingToolApprovals(approvals: Approval[], sessionId: string): PendingToolApproval[] {
  const id = sessionId.trim()
  if (!id) return []
  return approvals
    .filter((a) => a.type === 'tool_permission' && a.status === 'pending' && a.tool_permission?.session_id === id)
    .map((a) => ({ id: a.id, requestedAt: a.requested_at, request: a.tool_permission as ToolPermissionRequest }))
    .sort((a, b) => a.requestedAt.localeCompare(b.requestedAt))
}
