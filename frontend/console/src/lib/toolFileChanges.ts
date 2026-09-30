// Files a native provider's tool call changed, from the chat stream's
// `file_change` events (#1032). Each event names one file and the tool call
// that changed it; the tool card shows them as the call finishes. CLI
// providers never send the event, and the turn's change card covers them.

export type ToolFileChangeOp = 'create' | 'modify' | 'delete'

export type ToolFileChangeHunk = {
  old_start: number
  old_lines: number
  new_start: number
  new_lines: number
  // Unified-diff lines with their prefix: ' ', '-', '+', or '\'.
  lines: string[]
}

export type ToolFileChange = {
  path: string
  op: ToolFileChangeOp
  additions: number
  deletions: number
  binary?: boolean
  truncated?: boolean
  hunks?: ToolFileChangeHunk[]
}

type FileChangeEvent = {
  path?: string
  op?: string
  additions?: number
  deletions?: number
  binary?: boolean
  truncated?: boolean
  hunks?: ToolFileChangeHunk[]
}

const knownOps: readonly string[] = ['create', 'modify', 'delete'] satisfies ToolFileChangeOp[]

// The server sends only the three ops; anything else reads as a modify.
function changeOp(value: unknown): ToolFileChangeOp {
  return typeof value === 'string' && knownOps.includes(value) ? (value as ToolFileChangeOp) : 'modify'
}

function count(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
}

/** Reads a `file_change` event; null when it names no file. */
export function fileChangeFromEvent(event: FileChangeEvent): ToolFileChange | null {
  const path = typeof event.path === 'string' ? event.path.trim() : ''
  if (!path) return null
  const change: ToolFileChange = {
    path,
    op: changeOp(event.op),
    additions: count(event.additions),
    deletions: count(event.deletions),
  }
  if (event.binary) change.binary = true
  if (event.truncated) change.truncated = true
  if (Array.isArray(event.hunks) && event.hunks.length > 0) {
    change.hunks = event.hunks.filter((h) => h && Array.isArray(h.lines))
  }
  return change
}

/** Adds a change to a call's list; a later change to the same path replaces it. */
export function mergeToolFileChange(list: ToolFileChange[] | undefined, change: ToolFileChange): ToolFileChange[] {
  const current = list ?? []
  const at = current.findIndex((c) => c.path === change.path)
  if (at < 0) return [...current, change]
  const next = [...current]
  next[at] = change
  return next
}

export function totalFileChanges(list: ToolFileChange[] | undefined): { files: number; additions: number; deletions: number } {
  const changes = list ?? []
  return {
    files: changes.length,
    additions: changes.reduce((sum, c) => sum + c.additions, 0),
    deletions: changes.reduce((sum, c) => sum + c.deletions, 0),
  }
}

/** The hunks as unified-diff text, for lib/diff.ts's parser. */
export function fileChangePatch(change: ToolFileChange): string {
  const out: string[] = []
  for (const hunk of change.hunks ?? []) {
    out.push(`@@ -${hunk.old_start},${hunk.old_lines} +${hunk.new_start},${hunk.new_lines} @@`, ...hunk.lines)
  }
  return out.length ? `${out.join('\n')}\n` : ''
}

// The Changes panel's status words, so the card reuses its strings.
const statusByOp: Record<string, string> = { create: 'added', modify: 'modified', delete: 'deleted' }

export function fileChangeStatus(op: string): string {
  return statusByOp[op] ?? op
}
