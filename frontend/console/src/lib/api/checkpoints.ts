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
  // Applied reverts, oldest first. A turn's diff never changes after a
  // revert, so this is what marks what was taken back.
  reverts?: RevertEntry[]
}

// A file to revert; no hunk IDs means the whole file.
export type RevertFile = {
  path: string
  hunk_ids?: string[]
}

// turn: the turn's own edits, merged into later ones. since: every file back
// to how it was before the turn.
export type RevertScope = Extract<CheckpointScope, 'turn' | 'since'>

export type RevertRequest = {
  scope?: RevertScope
  files?: RevertFile[]
  // Without apply the result is a preview and nothing is written.
  apply?: boolean
  // Write over conflicts with later edits.
  force?: boolean
}

export type RevertOutcome = 'write' | 'merge' | 'unchanged' | 'conflict' | 'failed'

export type RevertFileResult = {
  path: string
  outcome: RevertOutcome | string
  delete?: boolean
  forced?: boolean
  // A conflicting merge with its markers.
  merged?: string
  detail?: string
}

export type RevertResult = {
  revert_id?: string
  turn_id: string
  scope: RevertScope
  applied: boolean
  conflicts: number
  failed: number
  files: RevertFileResult[]
}

export type RevertEntry = {
  id: string
  turn_id: string
  scope: RevertScope
  at: string
  targets: RevertFile[]
  files: string[]
  undone_at?: string
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

export function revertCheckpoint(sessionId: string, turnId: string, request: RevertRequest): Promise<RevertResult> {
  return requestJSON<RevertResult>(`${checkpointsPath(sessionId)}/${encodeURIComponent(turnId)}/revert`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  })
}

export function undoRevert(sessionId: string, revertId: string, force = false): Promise<RevertResult> {
  return requestJSON<RevertResult>(`${checkpointsPath(sessionId)}/reverts/${encodeURIComponent(revertId)}/undo`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ force }),
  })
}

// A note from reviewing an earlier turn's changes, sent with the next chat
// message: a comment on a hunk or file, or word that the user reverted it.
export type ReviewNote = {
  turn_id: string
  path: string
  hunk_id?: string
  comment?: string
  kind?: 'comment' | 'revert'
}
