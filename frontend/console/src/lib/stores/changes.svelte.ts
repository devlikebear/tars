// What each chat turn changed on disk (#969), shared by the Changes panel and
// the inline cards under each turn so both read one list.
//
// Turns are keyed by their user message's ID, which the thread knows from
// history (`SessionMessage.id`) or, for a live turn, from the stream's
// `turn_started` event. The API is injected so the store runs under plain
// Node in tests.

import type { getCheckpointDiff, listCheckpoints } from '../api'
import type { CheckpointDiff, CheckpointEntry, CheckpointEvent, CheckpointScope } from '../api/checkpoints'

export type ChangesApi = {
  listCheckpoints: typeof listCheckpoints
  getCheckpointDiff: typeof getCheckpointDiff
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

  private api: ChangesApi
  private diffs = new Map<string, Promise<CheckpointDiff>>()
  private loadSeq = 0

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
      this.error = ''
    } catch (err) {
      if (seq !== this.loadSeq) return
      this.error = err instanceof Error ? err.message : String(err)
    } finally {
      if (seq === this.loadSeq) this.loading = false
    }
  }
}
