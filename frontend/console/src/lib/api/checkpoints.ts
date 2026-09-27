import { requestJSON } from './client.ts'

// --- Turn checkpoints (#969) ---
//
// The server snapshots a chat turn's working folder before and after the
// model runs, whichever provider made the edits. A turn is keyed by its user
// message's ID.

// Why a turn has no checkpoint (internal/checkpoint Skip* reasons).
export type CheckpointSkipReason = 'too_many_files' | 'too_large' | 'failed'

export type CheckpointEntry = {
  turn_id: string
  // The work tree recorded: the repository's top level, or the session's
  // folder when that is not in a repository.
  root: string
  shadow: string
  start?: string
  end?: string
  started_at: string
  ended_at?: string
  // The start of the user's message.
  preview?: string
  files: number
  additions: number
  deletions: number
  // Paths left out of the snapshots (too large, unreadable, nested repos).
  unknown?: string[]
  skipped?: CheckpointSkipReason | string
  skip_detail?: string
}

export type CheckpointList = {
  session_id: string
  turns: CheckpointEntry[]
}

// turn: what the turn changed. session: everything the session's turns
// changed up to this one. since: this turn's start against the folder now.
export type CheckpointScope = 'turn' | 'session' | 'since'

export type CheckpointHunk = {
  id: string
  old_start: number
  old_lines: number
  new_start: number
  new_lines: number
  header: string
}

export type CheckpointFileStatus = 'added' | 'modified' | 'deleted' | 'renamed'

export type CheckpointFileDiff = {
  path: string
  old_path?: string
  status: CheckpointFileStatus | string
  additions: number
  deletions: number
  binary?: boolean
  patch?: string
  truncated?: boolean
  hunks?: CheckpointHunk[]
}

export type CheckpointDiff = {
  turn_id: string
  scope: CheckpointScope
  root: string
  from: string
  to: string
  files: CheckpointFileDiff[]
  unknown?: string[]
}

// The `checkpoint` chat stream event: a turn's end snapshot was taken.
export type CheckpointEvent = {
  session_id: string
  user_message_id: string
  files: number
  additions: number
  deletions: number
  skipped?: string
}

function checkpointsPath(sessionId: string): string {
  return `/v1/admin/sessions/${encodeURIComponent(sessionId)}/checkpoints`
}

export function listCheckpoints(sessionId: string): Promise<CheckpointList> {
  return requestJSON<CheckpointList>(checkpointsPath(sessionId))
}

export function getCheckpointDiff(
  sessionId: string,
  turnId: string,
  options: { scope?: CheckpointScope; path?: string } = {},
): Promise<CheckpointDiff> {
  const params = new URLSearchParams()
  if (options.scope) params.set('scope', options.scope)
  if (options.path) params.set('path', options.path)
  const suffix = params.toString()
  return requestJSON<CheckpointDiff>(`${checkpointsPath(sessionId)}/${encodeURIComponent(turnId)}/diff${suffix ? `?${suffix}` : ''}`)
}
