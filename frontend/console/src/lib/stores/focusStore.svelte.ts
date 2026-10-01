// Focus mode's pipeline screen state (docs/decisions/focus-mode.md, P1b).
//
// One store per open pipeline screen. It is rebuilt from the server every
// time — GET pipeline, session, transcript and checkpoints, then the turn
// feed (GET /v1/chat/stream) replayed from its first event — so a reload or
// a switch back mid-turn shows the same screen, with no duplicate cards:
// cards are the server's (replaced wholesale, never appended) plus change
// cards derived from checkpoints.
//
// next_prompt (from a gate/card API response or the chat stream's
// `pipeline` event) is sent as the next chat turn once the running turn
// ends. Each prompt is keyed by the pipeline update that produced it and
// remembered in storage, so a replayed event or a second look never sends it
// twice. P2 moves this to the server.
import { changeCards, deckFor, progressLine, turnIndex, turnStage, type ChangeTurn } from '../focus.ts'
import type { FocusTranslations } from '../../i18n/sections/focus.ts'
import type {
  ChatEvent,
  ChatRequest,
  FocusActionResult,
  FocusCard,
  FocusCardState,
  FocusGateAction,
  FocusPipeline,
  FocusPlan,
  FocusStageId,
  Session,
  SessionMessage,
} from '../types.ts'
import type { CheckpointDiff, CheckpointList } from '../api/checkpoints.ts'

export type FocusStoreApi = {
  getPipeline(sessionId: string): Promise<FocusPipeline>
  getSession(sessionId: string): Promise<Session>
  getHistory(sessionId: string): Promise<SessionMessage[]>
  listCheckpoints(sessionId: string): Promise<CheckpointList>
  getCheckpointDiff(sessionId: string, turnId: string, options?: { scope?: 'turn' }): Promise<CheckpointDiff>
  streamChat(request: ChatRequest, onEvent: (event: ChatEvent) => void, signal?: AbortSignal): Promise<void>
  attachChatStream(sessionId: string, onEvent: (event: ChatEvent) => void, signal?: AbortSignal): Promise<boolean>
  gate(sessionId: string, gate: string, action: FocusGateAction, options: { note?: string; edits?: FocusPlan }): Promise<FocusActionResult>
  card(sessionId: string, cardId: string, state: FocusCardState, decision?: string): Promise<FocusActionResult>
  advance(sessionId: string, stage: FocusStageId): Promise<FocusActionResult>
  stop(sessionId: string): Promise<FocusActionResult>
}

export type FocusStorage = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

// Change diffs fetched per load: the most recent turns that changed files.
const changeTurnLimit = 12
const sentKeysKept = 50
const turnEventsKept = 2000

function statusOf(err: unknown): number | undefined {
  const status = (err as { status?: unknown } | null)?.status
  return typeof status === 'number' ? status : undefined
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function time(value: string | undefined): number {
  const ms = Date.parse(value ?? '')
  return Number.isNaN(ms) ? 0 : ms
}

export class FocusStore {
  sessionId = $state<string | null>(null)
  pipeline = $state<FocusPipeline | null>(null)
  session = $state<Session | null>(null)
  history = $state<SessionMessage[]>([])
  changes = $state<FocusCard[]>([])
  loading = $state(false)
  // '' | 'notFound' | an error message from loading.
  error = $state('')
  // '' | 'stale' (a 409 replaced the state) | a server warning.
  notice = $state('')
  // An action or send failed; shown until the next action.
  actionError = $state('')
  // A turn runs: one this store sent, or one it follows through the feed.
  running = $state(false)
  turnEvents = $state<ChatEvent[]>([])
  busy = $state(false)
  // The stage whose cards the deck shows; null follows the current stage.
  viewStage = $state<FocusStageId | null>(null)

  private api: FocusStoreApi
  private storage: FocusStorage | null
  private pending: string[] = []
  private streaming = false
  private attaching: Promise<void> | null = null
  private controller: AbortController | null = null
  private diffCache = new Map<string, CheckpointDiff>()
  private localSeen = new Set<string>()

  constructor(api: FocusStoreApi, storage: FocusStorage | null = null) {
    this.api = api
    this.storage = storage
  }

  // All cards: the server's and the change cards.
  get cards(): FocusCard[] {
    return [...(this.pipeline?.cards ?? []), ...this.changes]
  }

  get stage(): FocusStageId | null {
    return this.viewStage ?? this.pipeline?.current ?? null
  }

  // The deck: the shown stage's cards in deck order.
  get deck(): FocusCard[] {
    const stage = this.stage
    return stage ? deckFor(this.cards, stage) : []
  }

  progress(stage?: FocusStageId, text?: FocusTranslations['progress']): string {
    return progressLine(this.turnEvents, { stage, text })
  }

  async load(sessionId: string): Promise<void> {
    if (this.sessionId !== sessionId) {
      this.controller?.abort()
      this.controller = null
      this.pipeline = null
      this.session = null
      this.history = []
      this.changes = []
      this.turnEvents = []
      this.running = false
      this.viewStage = null
      this.pending = []
      this.diffCache.clear()
      this.localSeen.clear()
    }
    this.sessionId = sessionId
    this.loading = true
    this.error = ''
    this.notice = ''
    this.actionError = ''
    try {
      const pipeline = await this.api.getPipeline(sessionId)
      if (this.sessionId !== sessionId) return
      this.pipeline = pipeline
    } catch (err) {
      if (this.sessionId !== sessionId) return
      this.error = statusOf(err) === 404 ? 'notFound' : message(err)
      this.loading = false
      return
    }
    const [session, history] = await Promise.all([
      this.api.getSession(sessionId).catch(() => null),
      this.api.getHistory(sessionId).catch(() => [] as SessionMessage[]),
    ])
    if (this.sessionId !== sessionId) return
    this.session = session
    // A session without messages answers `null`.
    this.history = history ?? []
    await this.refreshChanges()
    this.loading = false
    if (!this.streaming) {
      this.attaching = this.follow(sessionId)
      await this.attaching
    }
    this.kickoff()
  }

  // follow attaches to the session's running turn, replaying it from its
  // first event. It returns once the replay has caught up or no turn runs;
  // the rest of the turn streams in the background.
  private follow(sessionId: string): Promise<void> {
    const controller = new AbortController()
    this.controller = controller
    let caughtUp: () => void = () => {}
    const ready = new Promise<void>((resolve) => { caughtUp = resolve })
    const run = async () => {
      let attached = false
      try {
        attached = await this.api.attachChatStream(sessionId, (event) => {
          if (!this.running) {
            this.running = true
            this.turnEvents = []
          }
          caughtUp()
          this.applyEvent(event)
        }, controller.signal)
      } catch {
        // A dropped feed ends the follow; the GET below shows where it stands.
        attached = true
      } finally {
        caughtUp()
      }
      if (controller.signal.aborted || this.sessionId !== sessionId) return
      if (attached) await this.afterTurn()
      this.running = false
      this.attaching = null
      await this.flush()
    }
    void run()
    return ready
  }

  // kickoff sends the goal as the first turn of a pipeline that has none.
  private kickoff() {
    const p = this.pipeline
    if (!p || this.running || this.streaming || this.history.length > 0 || p.cards.length > 0) return
    if (p.current !== 'plan' || !p.goal.trim()) return
    this.queuePrompt(`first\n${p.session_id}`, p.goal)
    void this.flush()
  }

  // applyEvent folds one chat stream event (sent or replayed) into the state.
  applyEvent(event: ChatEvent) {
    if (event.session_id && this.sessionId && event.session_id !== this.sessionId) return
    if (event.type === 'turn_started') {
      this.turnEvents = [event]
    } else {
      this.turnEvents = [...this.turnEvents.slice(-turnEventsKept), event]
    }
    if (event.type === 'pipeline' && event.pipeline) {
      this.adopt(event.pipeline)
      if (event.next_prompt) this.queuePrompt(this.promptKey(event.pipeline, event.next_prompt), event.next_prompt)
    }
  }

  // adopt takes a pipeline unless it is older than the one held.
  private adopt(p: FocusPipeline, force = false) {
    if (p.session_id && this.sessionId && p.session_id !== this.sessionId) return
    if (!force && this.pipeline && time(p.updated_at) < time(this.pipeline.updated_at)) return
    this.pipeline = p
  }

  private promptKey(p: FocusPipeline, prompt: string): string {
    return `${p.updated_at}\n${prompt}`
  }

  private sentKeysName(): string {
    return `tars.focus.sent.${this.sessionId}`
  }

  private sentKeys(): string[] {
    try {
      const parsed = JSON.parse(this.storage?.getItem(this.sentKeysName()) ?? '[]')
      return Array.isArray(parsed) ? parsed.filter((k): k is string => typeof k === 'string') : []
    } catch {
      return []
    }
  }

  private queuePrompt(key: string, prompt: string) {
    const keys = this.sentKeys()
    if (keys.includes(key) || this.pending.includes(prompt)) return
    try {
      this.storage?.setItem(this.sentKeysName(), JSON.stringify([...keys, key].slice(-sentKeysKept)))
    } catch {
      // Storage full or blocked: the in-memory queue still dedupes this page.
    }
    this.pending.push(prompt)
  }

  private async flush() {
    if (this.streaming || this.running || this.attaching) return
    const next = this.pending.shift()
    if (next) await this.send(next)
  }

  // send runs one chat turn in the session — an instruction the developer
  // typed, or a next_prompt.
  async send(text: string): Promise<void> {
    const sessionId = this.sessionId
    const body = text.trim()
    if (!sessionId || !body) return
    if (this.streaming) {
      this.pending.push(body)
      return
    }
    this.streaming = true
    this.running = true
    this.turnEvents = []
    this.actionError = ''
    try {
      await this.api.streamChat({ message: body, session_id: sessionId }, (event) => this.applyEvent(event))
    } catch (err) {
      if (this.sessionId === sessionId) this.actionError = message(err)
    } finally {
      this.streaming = false
    }
    if (this.sessionId !== sessionId) return
    await this.afterTurn()
    this.running = false
    await this.flush()
  }

  // afterTurn re-reads what a finished turn changed.
  private async afterTurn() {
    const sessionId = this.sessionId
    if (!sessionId) return
    const [pipeline, history] = await Promise.all([
      this.api.getPipeline(sessionId).catch(() => null),
      this.api.getHistory(sessionId).then((h) => h ?? []).catch(() => null),
    ])
    if (this.sessionId !== sessionId) return
    if (pipeline) this.adopt(pipeline)
    if (history !== null) this.history = history ?? []
    await this.refreshChanges()
  }

  // refreshChanges rebuilds the change cards from the session's checkpoints:
  // one card per file of each recent turn that changed files. A turn's diff
  // never changes, so it is fetched once.
  async refreshChanges(): Promise<void> {
    const sessionId = this.sessionId
    if (!sessionId) return
    let list: CheckpointList
    try {
      list = await this.api.listCheckpoints(sessionId)
    } catch {
      return
    }
    const turns = (list?.turns ?? []).filter((t) => t.files > 0 && !t.skipped).slice(-changeTurnLimit)
    const out: ChangeTurn[] = []
    for (const entry of turns) {
      let diff = this.diffCache.get(entry.turn_id)
      if (!diff) {
        try {
          diff = await this.api.getCheckpointDiff(sessionId, entry.turn_id, { scope: 'turn' })
          this.diffCache.set(entry.turn_id, diff)
        } catch {
          continue
        }
      }
      const turn = turnIndex(this.history, entry.turn_id)
      const user = this.history.find((m) => m.id === entry.turn_id)
      out.push({
        turnId: entry.turn_id,
        turn,
        stage: (user && turnStage(user.content)) || this.pipeline?.current || 'build',
        at: entry.ended_at || entry.started_at,
        files: diff.files,
      })
    }
    if (this.sessionId !== sessionId) return
    const acked = new Set(this.acks())
    this.changes = changeCards(out, acked).map((c) => (c.state === 'unseen' && this.localSeen.has(c.id) ? { ...c, state: 'seen' } : c))
  }

  private acksName(): string {
    return `tars.focus.acks.${this.sessionId}`
  }

  private acks(): string[] {
    try {
      const parsed = JSON.parse(this.storage?.getItem(this.acksName()) ?? '[]')
      return Array.isArray(parsed) ? parsed.filter((k): k is string => typeof k === 'string') : []
    } catch {
      return []
    }
  }

  private markChange(id: string, state: FocusCardState) {
    if (state === 'decided') {
      const acks = this.acks()
      if (!acks.includes(id)) {
        try {
          this.storage?.setItem(this.acksName(), JSON.stringify([...acks, id]))
        } catch {
          // Kept in memory for this page.
        }
      }
    } else {
      this.localSeen.add(id)
    }
    this.changes = this.changes.map((c) => (c.id === id && c.state !== 'decided' ? { ...c, state } : c))
  }

  private async act(run: () => Promise<FocusActionResult>): Promise<boolean> {
    this.busy = true
    this.actionError = ''
    try {
      const result = await run()
      if (result.conflict) {
        this.adopt(result.pipeline, true)
        this.notice = 'stale'
        return false
      }
      this.adopt(result.pipeline, true)
      this.notice = result.warning ?? ''
      if (result.next_prompt) this.queuePrompt(this.promptKey(result.pipeline, result.next_prompt), result.next_prompt)
      return true
    } catch (err) {
      this.actionError = message(err)
      return false
    } finally {
      this.busy = false
    }
  }

  async gate(gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan): Promise<boolean> {
    const sessionId = this.sessionId
    if (!sessionId) return false
    const ok = await this.act(() => this.api.gate(sessionId, gate, action, { note, edits }))
    if (ok) this.viewStage = null
    await this.flush()
    return ok
  }

  async markCard(cardId: string, state: FocusCardState, decision?: string): Promise<boolean> {
    const sessionId = this.sessionId
    if (!sessionId) return false
    if (cardId.startsWith('change:')) {
      this.markChange(cardId, state)
      return true
    }
    const ok = await this.act(() => this.api.card(sessionId, cardId, state, decision))
    await this.flush()
    return ok
  }

  // markSeen notes that a card was on screen. Cards the developer must
  // handle stay unseen so they keep counting as needing input.
  async markSeen(card: FocusCard): Promise<void> {
    if (card.state !== 'unseen' || card.kind === 'gate' || card.kind === 'decision' || card.kind === 'finding') return
    if (card.id.startsWith('change:')) {
      this.markChange(card.id, 'seen')
      return
    }
    const sessionId = this.sessionId
    if (!sessionId) return
    try {
      const result = await this.api.card(sessionId, card.id, 'seen')
      this.adopt(result.pipeline)
    } catch {
      // Seen is a courtesy; a failure changes nothing the developer relies on.
    }
  }

  // acknowledgeRest decides every remaining report, change, failure and
  // notice card of the deck.
  async acknowledgeRest(cards: FocusCard[]): Promise<void> {
    for (const card of cards) {
      if (card.state === 'decided') continue
      if (card.kind !== 'report' && card.kind !== 'change' && card.kind !== 'failure' && card.kind !== 'notice') continue
      await this.markCard(card.id, 'decided', 'acknowledged')
    }
  }

  async advance(): Promise<boolean> {
    const sessionId = this.sessionId
    const stage = this.pipeline?.current
    if (!sessionId || !stage) return false
    const ok = await this.act(() => this.api.advance(sessionId, stage))
    if (ok) this.viewStage = null
    await this.flush()
    return ok
  }

  async stop(): Promise<boolean> {
    const sessionId = this.sessionId
    if (!sessionId) return false
    return this.act(() => this.api.stop(sessionId))
  }

  showStage(stage: FocusStageId | null) {
    this.viewStage = stage && stage !== this.pipeline?.current ? stage : null
  }

  dispose() {
    this.controller?.abort()
    this.controller = null
  }
}
