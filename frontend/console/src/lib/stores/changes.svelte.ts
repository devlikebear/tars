// What each chat turn changed on disk (#969), shared by the Changes panel and
// the inline cards under each turn so both read one list.
//
// Turns are keyed by their user message's ID, which the thread knows from
// history (`SessionMessage.id`) or, for a live turn, from the stream's
// `turn_started` event. The API is injected so the store runs under plain
// Node in tests.

import type { getCheckpointDiff, listCheckpoints, revertCheckpoint, undoRevert } from '../api'
import type {
  CheckpointDiff,
  CheckpointEntry,
  CheckpointEvent,
  CheckpointScope,
  RevertEntry,
  RevertFile,
  ReviewNote,
  RevertResult,
  RevertScope,
} from '../api/checkpoints'

export type ChangesApi = {
  listCheckpoints: typeof listCheckpoints
  getCheckpointDiff: typeof getCheckpointDiff
  revertCheckpoint: typeof revertCheckpoint
  undoRevert: typeof undoRevert
}

// A revert being set up: previewed, then confirmed (or forced over
// conflicts), then applied.
export type PendingRevert = {
  turnId: string
  scope: RevertScope
  files: RevertFile[]
  stage: 'checking' | 'confirm' | 'conflict' | 'applying' | 'error'
  result: RevertResult | null
  // The server's error code (turn_in_progress, …) or '' with a message.
  errorCode: string
  error: string
}

// The last applied revert, kept for its Undo.
export type LastRevert = {
  revertId: string
  turnId: string
  result: RevertResult
  stage: 'done' | 'undoing' | 'undoConflict' | 'undone' | 'error'
  undoResult: RevertResult | null
  errorCode: string
  error: string
}

// A review note waiting for the next message. revertId ties a revert's note
// to that revert, so undoing it takes the note back.
export type DraftNote = ReviewNote & { id: string; revertId?: string }

type ErrorShape = { status?: number; message?: string; payload?: { code?: string; result?: RevertResult } }

// A 409 revert_conflict carries the result, so the conflicts can be shown.
function conflictOf(err: unknown): RevertResult | null {
  const e = err as ErrorShape
  return e?.status === 409 && e.payload?.code === 'revert_conflict' && e.payload.result ? e.payload.result : null
}

function errorFields(err: unknown): { errorCode: string; error: string } {
  const e = err as ErrorShape
  return { errorCode: e?.payload?.code ?? '', error: e?.message ?? String(err) }
}

// The panel's two views: one turn, or everything the session changed up to it.
export type ChangesScope = Extract<CheckpointScope, 'turn' | 'session'>

export class ChangesStore {
  sessionId = $state<string | null>(null)
  // Oldest first, as the server keeps them.
  turns = $state<CheckpointEntry[]>([])
  loading = $state(false)
  error = $state('')
  // The turn the Changes panel shows; null follows the latest changed turn.
  selectedTurnId = $state<string | null>(null)
  scope = $state<ChangesScope>('turn')
  // Bumped whenever cached diffs are dropped, so views reload theirs.
  version = $state(0)
  reverts = $state<RevertEntry[]>([])
  pending = $state<PendingRevert | null>(null)
  last = $state<LastRevert | null>(null)
  // Notes for the next chat message.
  notes = $state<DraftNote[]>([])
  private noteSeq = 0

  private api: ChangesApi
  private diffs = new Map<string, Promise<CheckpointDiff>>()
  private loadSeq = 0
  // Which revert request is current: $state proxies objects, so identity
  // checks against the object stored cannot tell them apart.
  private revertSeq = 0

  constructor(api: ChangesApi) {
    this.api = api
  }

  // Turns that changed something, newest first.
  get changedTurns(): CheckpointEntry[] {
    return this.turns.filter((turn) => turn.files > 0).reverse()
  }

  get selectedTurn(): CheckpointEntry | undefined {
    if (this.selectedTurnId) {
      const picked = this.turn(this.selectedTurnId)
      if (picked) return picked
    }
    return this.changedTurns[0]
  }

  turn(turnId: string | undefined): CheckpointEntry | undefined {
    if (!turnId) return undefined
    return this.turns.find((turn) => turn.turn_id === turnId)
  }

  // Switch to a session's turns. Loading the one already shown refreshes it.
  async load(sessionId: string | null | undefined): Promise<void> {
    const next = sessionId?.trim() || null
    if (next !== this.sessionId) {
      this.sessionId = next
      this.turns = []
      this.reverts = []
      this.pending = null
      this.last = null
      this.notes = []
      this.error = ''
      this.selectedTurnId = null
      this.dropDiffs(() => true)
    }
    await this.fetchTurns()
  }

  // Reload the list. Diffs that read the folder as it is now, or span later
  // turns, are dropped; a finished turn's own diff cannot change.
  async refresh(): Promise<void> {
    this.dropDiffs((key) => !key.endsWith(':turn:'))
    await this.fetchTurns()
  }

  // A turn's end snapshot arrived on the chat stream. The thread belongs to
  // this session even when it was created by that very turn.
  applyEvent(event: CheckpointEvent): void {
    const sessionId = event.session_id?.trim()
    const turnId = event.user_message_id?.trim()
    if (!sessionId || !turnId) return
    if (sessionId !== this.sessionId) {
      this.sessionId = sessionId
      this.turns = []
      this.reverts = []
      this.pending = null
      this.last = null
      this.notes = []
      this.selectedTurnId = null
      this.dropDiffs(() => true)
    }
    const summary = {
      files: event.files ?? 0,
      additions: event.additions ?? 0,
      deletions: event.deletions ?? 0,
      skipped: event.skipped || undefined,
    }
    const index = this.turns.findIndex((turn) => turn.turn_id === turnId)
    if (index >= 0) {
      this.turns[index] = { ...this.turns[index], ...summary }
    } else {
      this.turns = [...this.turns, { turn_id: turnId, root: '', shadow: '', started_at: new Date().toISOString(), ...summary }]
    }
    this.dropDiffs((key) => key.startsWith(`${turnId}:`) || !key.endsWith(':turn:'))
    void this.fetchTurns()
  }

  select(turnId: string | null): void {
    this.selectedTurnId = turnId
  }

  setScope(scope: ChangesScope): void {
    this.scope = scope
  }

  // One turn's diff, fetched once per turn, scope, and path.
  diff(turnId: string, scope: CheckpointScope = 'turn', path = ''): Promise<CheckpointDiff> {
    const sessionId = this.sessionId
    if (!sessionId) return Promise.reject(new Error('no session'))
    const key = `${turnId}:${scope}:${path}`
    let pending = this.diffs.get(key)
    if (!pending) {
      pending = this.api.getCheckpointDiff(sessionId, turnId, { scope, path: path || undefined })
      this.diffs.set(key, pending)
      pending.catch(() => {
        if (this.diffs.get(key) === pending) this.diffs.delete(key)
      })
    }
    return pending
  }

  // Whether a live revert took back this turn's file, or one of its hunks.
  // Without a hunk, only a whole-file revert counts.
  isReverted(turnId: string, path: string, hunkId?: string): boolean {
    return this.reverts.some((r) => !r.undone_at && r.scope === 'turn' && r.turn_id === turnId && r.targets.some((target) => {
      if (target.path !== path) return false
      if (!target.hunk_ids?.length) return true
      return !!hunkId && target.hunk_ids.includes(hunkId)
    }))
  }

  // Preview a revert; the user then confirms it, or forces it over conflicts.
  async requestRevert(turnId: string, scope: RevertScope, files: RevertFile[] = []): Promise<void> {
    const sessionId = this.sessionId
    if (!sessionId) return
    const pending: PendingRevert = { turnId, scope, files, stage: 'checking', result: null, errorCode: '', error: '' }
    const seq = ++this.revertSeq
    this.pending = pending
    try {
      const result = await this.api.revertCheckpoint(sessionId, turnId, { scope, files })
      if (seq !== this.revertSeq) return
      this.pending = { ...pending, result, stage: result.conflicts > 0 ? 'conflict' : 'confirm' }
    } catch (err) {
      if (seq !== this.revertSeq) return
      this.pending = { ...pending, stage: 'error', ...errorFields(err) }
    }
  }

  cancelRevert(): void {
    this.revertSeq++
    this.pending = null
  }

  // Apply the pending revert. Whatever happens, the list is reloaded: even a
  // failed apply may have written some files.
  async confirmRevert(force = false): Promise<void> {
    const sessionId = this.sessionId
    const pending = this.pending
    if (!sessionId || !pending || pending.stage === 'applying' || pending.stage === 'checking') return
    const applying: PendingRevert = { ...pending, stage: 'applying', errorCode: '', error: '' }
    const seq = ++this.revertSeq
    this.pending = applying
    try {
      const result = await this.api.revertCheckpoint(sessionId, pending.turnId, { scope: pending.scope, files: pending.files, apply: true, force })
      if (seq === this.revertSeq) this.pending = null
      this.last = { revertId: result.revert_id ?? '', turnId: pending.turnId, result, stage: 'done', undoResult: null, errorCode: '', error: '' }
      if (result.revert_id) this.noteRevert(pending.turnId, result.revert_id, pending, result)
    } catch (err) {
      if (seq === this.revertSeq) {
        const conflict = conflictOf(err)
        this.pending = conflict
          ? { ...applying, stage: 'conflict', result: conflict }
          : { ...applying, stage: 'error', ...errorFields(err) }
      }
    } finally {
      await this.refresh()
    }
  }

  // Put the last revert back; force overwrites files edited since.
  async undo(force = false): Promise<void> {
    const sessionId = this.sessionId
    const last = this.last
    if (!sessionId || !last?.revertId || last.stage === 'undoing' || last.stage === 'undone') return
    this.last = { ...last, stage: 'undoing', errorCode: '', error: '' }
    try {
      const undoResult = await this.api.undoRevert(sessionId, last.revertId, force)
      this.last = { ...last, stage: 'undone', undoResult, errorCode: '', error: '' }
      this.notes = this.notes.filter((note) => note.revertId !== last.revertId)
    } catch (err) {
      const conflict = conflictOf(err)
      this.last = conflict
        ? { ...last, stage: 'undoConflict', undoResult: conflict, errorCode: '', error: '' }
        : { ...last, stage: 'error', ...errorFields(err) }
    } finally {
      await this.refresh()
    }
  }

  dismissLast(): void {
    this.last = null
  }

  addNote(note: ReviewNote, revertId?: string): void {
    this.notes = [...this.notes, { ...note, id: `n${++this.noteSeq}`, revertId }]
  }

  removeNote(id: string): void {
    this.notes = this.notes.filter((note) => note.id !== id)
  }

  // The notes to send with a message, as the server takes them. They are
  // cleared: once sent they are part of the message.
  takeNotes(): ReviewNote[] {
    const wire = this.notes.map(({ turn_id, path, hunk_id, comment, kind }) => {
      const note: ReviewNote = { turn_id, path }
      if (hunk_id) note.hunk_id = hunk_id
      if (comment) note.comment = comment
      if (kind) note.kind = kind
      return note
    })
    this.notes = []
    return wire
  }

  // A revert tells the agent what it took back, so the next turn does not
  // redo it.
  private noteRevert(turnId: string, revertId: string, pending: PendingRevert, result: RevertResult): void {
    const written = new Set(result.files.filter((f) => f.outcome === 'write' || f.outcome === 'merge').map((f) => f.path))
    // Named files keep their hunks; a whole-turn or since revert names what
    // it wrote.
    const targets: RevertFile[] = pending.scope === 'turn' && pending.files.length > 0
      ? pending.files.filter((file) => written.has(file.path))
      : [...written].map((path) => ({ path }))
    for (const file of targets) {
      const hunks = file.hunk_ids?.length ? file.hunk_ids : [undefined]
      for (const hunk_id of hunks) this.addNote({ turn_id: turnId, path: file.path, hunk_id, kind: 'revert' }, revertId)
    }
  }

  private dropDiffs(match: (key: string) => boolean): void {
    let dropped = false
    for (const key of [...this.diffs.keys()]) {
      if (match(key)) {
        this.diffs.delete(key)
        dropped = true
      }
    }
    if (dropped) this.version++
  }

  private async fetchTurns(): Promise<void> {
    const sessionId = this.sessionId
    const seq = ++this.loadSeq
    if (!sessionId) {
      this.loading = false
      return
    }
    this.loading = true
    try {
      const result = await this.api.listCheckpoints(sessionId)
      if (seq !== this.loadSeq) return
      this.turns = result.turns ?? []
      this.reverts = result.reverts ?? []
      this.error = ''
    } catch (err) {
      if (seq !== this.loadSeq) return
      this.error = err instanceof Error ? err.message : String(err)
    } finally {
      if (seq === this.loadSeq) this.loading = false
    }
  }
}
