// Focus mode's pipeline screen state (docs/decisions/focus-mode.md, P1b).
//
// One store per open pipeline screen. It is rebuilt from the server every
// time — GET pipeline, session, transcript and checkpoints, then the turn
// feed (GET /v1/chat/stream) replayed from its first event — so a reload or
// a switch back mid-turn shows the same screen, with no duplicate cards:
// cards are the server's (replaced wholesale, never appended) plus change
// cards derived from checkpoints.
//
// The server carries the pipeline forward (P2): after a turn or a gate,
// card or advance action it runs verification and sends the next turn
// itself. next_prompt (from an action's response or the chat stream's
// `pipeline` event) is only shown; this store follows the server's work on
// GET /v1/chat/stream like any turn started elsewhere.
//
// What the developer sends — the first message (the goal, or a release's
// kickoff text) and typed instructions — goes
// through one queue, persisted in storage, sent one at a time when no turn
// runs on the session (here, in another tab, in Advanced, or the server's —
// /v1/chat/activity). Each entry is keyed and marked sent only once the
// server accepted the chat request, so a reload or a failed request never
// loses one and none is sent twice.
//
// Q&A (ADR §8): questions about a card go to the pipeline's hidden Q&A
// session; answers stream on that session's feed and thread by card from
// its history and the pipeline's qa_turns.
import { changeCards, deckFor, openGateCardId, progressLine, qaThreads, stageKindOf, turnIndex, turnStage, type ChangeTurn, type QAEntry } from '../focus.ts'
import { takeKickoffAttachments } from '../focusKickoff.ts'
import type { FocusTranslations } from '../../i18n/sections/focus.ts'
import type {
  ChatAttachment,
  ChatEvent,
  ChatRequest,
  FocusActionResult,
  FocusCard,
  FocusCardState,
  FocusFindingInput,
  FocusGateAction,
  FocusPipeline,
  FocusPlan,
  FocusPRDraft,
  FocusQAResult,
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
  gate(sessionId: string, gate: string, action: FocusGateAction, options: { note?: string; edits?: FocusPlan; pr?: FocusPRDraft; card_id?: string }): Promise<FocusActionResult>
  card(sessionId: string, cardId: string, state: FocusCardState, decision?: string): Promise<FocusActionResult>
  advance(sessionId: string, stage: FocusStageId): Promise<FocusActionResult>
  // The developer's own finding (POST …/findings).
  addFinding?(sessionId: string, finding: FocusFindingInput): Promise<FocusActionResult>
  stop(sessionId: string): Promise<FocusActionResult>
  // Goal mode on or off (POST …/goal).
  goal?(sessionId: string, enabled: boolean): Promise<FocusActionResult>
  ask?(sessionId: string, cardId: string, question: string): Promise<FocusQAResult>
  // Turns running now, across sessions (GET /v1/chat/activity).
  activity?(): Promise<{ running?: { session_id: string }[] | null }>
}

// attachments rides only on the first turn (the goal's pasted images); a
// typed instruction never carries them, so the field is optional and
// omitted for every other entry.
type QueuedPrompt = { key: string; prompt: string; attachments?: ChatAttachment[] }

export type FocusStorage = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

// Change diffs fetched per load: the most recent turns that changed files.
const changeTurnLimit = 12
const sentKeysKept = 50
const turnEventsKept = 2000
// After an action the server starts its turn within moments: look for it
// chaseTries times, chaseDelayMs apart, before leaving it to the screen's
// poll. A Q&A answer's feed likewise appears once its turn starts.
// A Q&A answer is followed until its turn ends — however long it waits to
// start — up to qaMaxWaitMs.
export const focusTimingDefaults = { chaseTries: 10, chaseDelayMs: 300, qaAttachTries: 25, qaAttachDelayMs: 200, qaMaxWaitMs: 15 * 60 * 1000 }

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

// The question being answered now, with the answer as it streams.
export type QAPending = { cardId: string; turn: number; question: string; answer: string }

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
  // The turn the server sent last on the pipeline's behalf (display only).
  nextPrompt = $state('')
  // The Q&A session's transcript, the question being answered, and the
  // last failure to ask.
  qaHistory = $state<SessionMessage[]>([])
  qaPending = $state<QAPending | null>(null)
  qaError = $state('')

  private api: FocusStoreApi
  private storage: FocusStorage | null
  private pending: QueuedPrompt[] = []
  // The transcript was read; until then nothing can tell a new pipeline.
  private historyKnown = false
  private disposed = false
  private flushing = false
  // A chat request failed before the server took it: stop retrying until
  // the next load or action, so a server that is down is not hammered.
  private sendBlocked = false
  private streaming = false
  private attaching: Promise<void> | null = null
  private controller: AbortController | null = null
  private diffCache = new Map<string, CheckpointDiff>()
  private localSeen = new Set<string>()
  private chasing = false
  // Overridable by tests.
  timing = { ...focusTimingDefaults }
  private qaController: AbortController | null = null

  constructor(api: FocusStoreApi, storage: FocusStorage | null = null) {
    this.api = api
    this.storage = storage
  }

  // All cards: the server's and the change cards.
  get cards(): FocusCard[] {
    return [...(this.pipeline?.cards ?? []), ...this.changes]
  }

  // The stage whose cards the deck shows: the one picked on the stepper, else
  // the current stage — or, while that has no cards yet (a turn just started
  // it), the latest earlier stage that has some, so they stay navigable.
  get stage(): FocusStageId | null {
    if (this.viewStage) return this.viewStage
    const p = this.pipeline
    if (!p) return null
    const cards = this.cards
    if (cards.some((c) => c.stage === p.current)) return p.current
    const at = p.stages.findIndex((s) => s.id === p.current)
    for (let i = at - 1; i >= 0; i--) {
      const id = p.stages[i].id
      if (cards.some((c) => c.stage === id)) return id
    }
    return p.current
  }

  // The deck: the shown stage's cards in deck order.
  get deck(): FocusCard[] {
    const stage = this.stage
    return stage ? deckFor(this.cards, stage) : []
  }

  // lead is a template stage's own name, shown instead of the development
  // stage's phrase.
  progress(stage?: FocusStageId, text?: FocusTranslations['progress'], lead?: string): string {
    return progressLine(this.turnEvents, { stage: stageKindOf(this.pipeline?.stages, stage) ?? undefined, text, lead })
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
      this.nextPrompt = ''
      this.qaHistory = []
      this.qaPending = null
      this.qaError = ''
      this.qaController?.abort()
      this.qaController = null
      this.historyKnown = false
      this.diffCache.clear()
      this.localSeen.clear()
    }
    this.sessionId = sessionId
    this.pending = this.storedPending()
    this.sendBlocked = false
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
      // A session without messages answers `null`; a failed read is unknown.
      this.api.getHistory(sessionId).then((h) => h ?? []).catch(() => null),
    ])
    if (this.sessionId !== sessionId) return
    this.session = session
    this.historyKnown = history !== null
    this.history = history ?? []
    await this.refreshChanges()
    void this.loadQA()
    this.loading = false
    if (!this.streaming && !this.attaching) {
      this.attaching = this.follow(sessionId)
      await this.attaching
    }
    this.kickoff()
    void this.flush()
  }

  // poll follows a turn that started elsewhere — another tab, a store this
  // screen replaced, or Advanced — so this screen shows it and never starts
  // a parallel one. The screen calls it on an interval.
  async poll(): Promise<void> {
    const sessionId = this.sessionId
    if (!sessionId || this.disposed || this.streaming || this.running || this.attaching) return
    if (!(await this.runningElsewhere(sessionId))) {
      // Nothing runs: re-read the pipeline — the server changes it without a
      // turn (the PR stages' gh probe, P4), and a turn short enough to end
      // between two activity checks is never followed — then send what
      // waited (a prompt queued while the activity still listed a turn that
      // was ending).
      await this.refreshPipeline(sessionId)
      await this.flush()
      return
    }
    if (this.sessionId !== sessionId || this.disposed || this.streaming || this.running || this.attaching) return
    this.attaching = this.follow(sessionId)
    await this.attaching
  }

  // refreshPipeline re-reads everything a finished turn changed when the
  // server's pipeline is newer than the one held: the pipeline alone would
  // carry cards from a turn whose transcript (view raw) is not loaded yet.
  private async refreshPipeline(sessionId: string): Promise<void> {
    const pipeline = await this.api.getPipeline(sessionId).catch(() => null)
    if (!pipeline || this.sessionId !== sessionId || this.disposed) return
    if (this.pipeline && time(pipeline.updated_at) <= time(this.pipeline.updated_at)) return
    await this.afterTurn()
  }

  private async runningElsewhere(sessionId: string): Promise<boolean> {
    if (!this.api.activity) return false
    try {
      const snap = await this.api.activity()
      return (snap?.running ?? []).some((r) => r.session_id === sessionId)
    } catch {
      return false
    }
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
            // The stream is back: an error from a dropped one is history (U3).
            this.actionError = ''
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
      if (controller.signal.aborted || this.disposed || this.sessionId !== sessionId) {
        if (this.controller === controller) this.attaching = null
        return
      }
      if (attached) await this.afterTurn()
      this.running = false
      this.attaching = null
      await this.flush()
      if (attached) void this.chase()
    }
    void run()
    return ready
  }

  // chase follows the server's next step of the pipeline (verification,
  // the next turn) as soon as it starts, instead of waiting for the screen's
  // poll. It gives up after a few tries: the poll still catches it.
  private async chase(): Promise<void> {
    if (this.chasing) return
    this.chasing = true
    try {
      for (let i = 0; i < this.timing.chaseTries; i++) {
        const p = this.pipeline
        if (this.disposed || !this.sessionId || !p || p.open_gate) return
        if (this.streaming || this.running || this.attaching) return
        await this.poll()
        if (this.running || this.attaching) return
        await sleep(this.timing.chaseDelayMs)
      }
    } finally {
      this.chasing = false
    }
  }

  // kickoff sends the goal (or the pipeline's kickoff text, when it has one)
  // as the first turn of a pipeline that has none. A pipeline created with
  // start: true (API, desktop) already owes that turn (pending_turn): the
  // server sends it, so this screen only follows it.
  private kickoff() {
    const p = this.pipeline
    if (!p || p.pending_turn || !this.historyKnown || this.running || this.streaming || this.history.length > 0 || p.cards.length > 0) return
    const first = p.kickoff?.trim() ? p.kickoff : p.goal
    if (p.current !== 'plan' || !first.trim()) return
    // Pasted into the goal field before the session existed (FocusNewTask);
    // picked up once, here, and never again for this session id.
    this.queuePrompt(`first\n${p.session_id}`, first, takeKickoffAttachments(p.session_id))
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
      // Shown, never sent: the server sends it (P2).
      if (event.next_prompt) this.nextPrompt = event.next_prompt
    }
  }

  // adopt takes a pipeline unless it is older than the one held.
  private adopt(p: FocusPipeline, force = false) {
    if (p.session_id && this.sessionId && p.session_id !== this.sessionId) return
    if (!force && this.pipeline && time(p.updated_at) < time(this.pipeline.updated_at)) return
    this.pipeline = p
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

  private pendingName(): string {
    return `tars.focus.pending.${this.sessionId}`
  }

  private storedPending(): QueuedPrompt[] {
    try {
      const parsed = JSON.parse(this.storage?.getItem(this.pendingName()) ?? '[]')
      if (!Array.isArray(parsed)) return []
      const sent = this.sentKeys()
      return parsed.filter(
        (e): e is QueuedPrompt =>
          typeof e?.key === 'string' && typeof e?.prompt === 'string' && (e.attachments === undefined || Array.isArray(e.attachments)) && !sent.includes(e.key),
      )
    } catch {
      return []
    }
  }

  private savePending() {
    try {
      this.storage?.setItem(this.pendingName(), JSON.stringify(this.pending))
    } catch {
      // Storage full or blocked: the queue lives in memory for this page.
    }
  }

  private queuePrompt(key: string, prompt: string, attachments?: ChatAttachment[]) {
    if (this.sentKeys().includes(key) || this.pending.some((e) => e.key === key)) return
    this.pending = [...this.pending, attachments && attachments.length > 0 ? { key, prompt, attachments } : { key, prompt }]
    this.savePending()
  }

  // markSent records that the server took an entry's chat request.
  private markSent(entry: QueuedPrompt) {
    try {
      const keys = this.sentKeys()
      if (!keys.includes(entry.key)) this.storage?.setItem(this.sentKeysName(), JSON.stringify([...keys, entry.key].slice(-sentKeysKept)))
    } catch {
      // The in-memory queue below still drops it for this page.
    }
    this.pending = this.pending.filter((e) => e.key !== entry.key)
    this.savePending()
  }

  // flush sends the next queued entry when no turn runs on the session.
  private async flush() {
    const sessionId = this.sessionId
    if (!sessionId || this.disposed || this.sendBlocked || this.flushing || this.streaming || this.running || this.attaching) return
    this.flushing = true
    try {
      // Another tab may have sent an entry since it was queued.
      const sent = this.sentKeys()
      this.pending = this.pending.filter((e) => !sent.includes(e.key))
      const next = this.pending[0]
      if (!next || (await this.runningElsewhere(sessionId))) return
      if (this.disposed || this.sessionId !== sessionId || this.streaming || this.running || this.attaching) return
      this.flushing = false
      await this.sendEntry(next)
    } finally {
      this.flushing = false
    }
  }

  // send queues an instruction the developer typed; it goes out at once when
  // no turn runs, else after it.
  async send(text: string): Promise<void> {
    const body = text.trim()
    if (!this.sessionId || !body || this.disposed) return
    this.queuePrompt(`typed\n${Date.now()}\n${body}`, body)
    this.sendBlocked = false
    await this.flush()
  }

  // sendEntry runs one chat turn for a queued entry.
  private async sendEntry(entry: QueuedPrompt): Promise<void> {
    const sessionId = this.sessionId
    if (!sessionId) return
    this.streaming = true
    this.running = true
    this.turnEvents = []
    this.actionError = ''
    // This turn is the developer's own: no pipeline prompt is running (f7).
    this.nextPrompt = ''
    let accepted = false
    const accept = () => {
      if (accepted) return
      accepted = true
      this.markSent(entry)
    }
    try {
      await this.api.streamChat({ message: entry.prompt, session_id: sessionId, attachments: entry.attachments }, (event) => {
        accept()
        if (!this.disposed) this.applyEvent(event)
      })
      accept()
    } catch (err) {
      if (!accepted && statusOf(err) === 409) {
        // Another turn took the session in the same instant (the server's):
        // the entry stays queued and goes out once that turn ends.
      } else {
        // Not accepted: the entry stays queued for the next load or action.
        if (!accepted) this.sendBlocked = true
        if (this.sessionId === sessionId && !this.disposed) this.actionError = message(err)
      }
    } finally {
      this.streaming = false
    }
    if (this.disposed || this.sessionId !== sessionId) return
    await this.afterTurn()
    this.running = false
    await this.flush()
    void this.chase()
  }

  // afterTurn re-reads what a finished turn changed.
  private async afterTurn() {
    const sessionId = this.sessionId
    if (!sessionId || this.disposed) return
    // The session too: a turn may have moved it into a worktree.
    const [pipeline, history, session] = await Promise.all([
      this.api.getPipeline(sessionId).catch(() => null),
      this.api.getHistory(sessionId).then((h) => h ?? []).catch(() => null),
      this.api.getSession(sessionId).catch(() => null),
    ])
    if (this.sessionId !== sessionId || this.disposed) return
    if (session) this.session = session
    if (pipeline) this.adopt(pipeline)
    if (history !== null) {
      this.history = history
      this.historyKnown = true
    }
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
    this.sendBlocked = false
    try {
      const result = await run()
      if (result.conflict) {
        this.adopt(result.pipeline, true)
        this.notice = 'stale'
        return false
      }
      this.adopt(result.pipeline, true)
      this.notice = result.warning ?? ''
      if (result.next_prompt) {
        // The server started this turn; follow it as soon as it shows.
        this.nextPrompt = result.next_prompt
        void this.chase()
      }
      return true
    } catch (err) {
      this.actionError = message(err)
      return false
    } finally {
      this.busy = false
    }
  }

  // pr is G3's edited title and body (P4).
  async gate(gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan, pr?: FocusPRDraft): Promise<boolean> {
    const sessionId = this.sessionId
    if (!sessionId) return false
    const options: { note?: string; edits?: FocusPlan; pr?: FocusPRDraft; card_id?: string } = pr ? { note, edits, pr } : { note, edits }
    // G4 shows one PR head's facts: name the card on screen so a G4 the
    // server reopened on a new head meanwhile is not approved (#1087).
    const shown = gate === 'merge' && this.pipeline ? openGateCardId(this.pipeline) : undefined
    if (shown) options.card_id = shown
    const ok = await this.act(() => this.api.gate(sessionId, gate, action, options))
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

  // addFinding adds the developer's own finding to the current review or
  // pr_review round. The server sends the fix turn it may ask for.
  async addFinding(finding: FocusFindingInput): Promise<boolean> {
    const sessionId = this.sessionId
    const add = this.api.addFinding
    if (!sessionId || !add) return false
    const ok = await this.act(() => add(sessionId, finding))
    if (ok) this.viewStage = null
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

  // setGoal turns the pipeline's goal mode on or off.
  async setGoal(enabled: boolean): Promise<boolean> {
    const sessionId = this.sessionId
    const goal = this.api.goal
    if (!sessionId || !goal) return false
    return this.act(() => goal(sessionId, enabled))
  }

  showStage(stage: FocusStageId | null) {
    this.viewStage = stage && stage !== this.pipeline?.current ? stage : null
  }

  // --- Q&A (ADR §8) ---

  // qaThread is the questions about a card and their answers, oldest
  // first, with the one being answered now last.
  qaThread(cardId: string): QAEntry[] {
    const thread = qaThreads(this.qaHistory, this.pipeline?.qa_turns)[cardId] ?? []
    const pending = this.qaPending
    if (pending && pending.cardId === cardId && !thread.some((e) => e.turn === pending.turn && e.answer)) {
      return [...thread.filter((e) => e.turn !== pending.turn), { turn: pending.turn, question: pending.question, answer: pending.answer }]
    }
    return thread
  }

  // loadQA reads the Q&A session's transcript, if the pipeline has one.
  private async loadQA(): Promise<void> {
    const qaId = this.pipeline?.qa_session_id
    const sessionId = this.sessionId
    if (!qaId || !sessionId) return
    try {
      const history = (await this.api.getHistory(qaId)) ?? []
      if (this.sessionId === sessionId && !this.disposed) this.qaHistory = history
    } catch {
      // The threads stay as they were; the next answer reads them again.
    }
  }

  // ask sends a question about a card to the Q&A session and streams its
  // answer. It never changes the pipeline's stage or cards.
  async ask(cardId: string, question: string): Promise<boolean> {
    const sessionId = this.sessionId
    const text = question.trim()
    if (!sessionId || !text || this.disposed || !this.api.ask) return false
    if (this.qaPending) {
      this.qaError = 'busy'
      return false
    }
    this.qaError = ''
    let result: FocusQAResult
    try {
      result = await this.api.ask(sessionId, cardId, text)
    } catch (err) {
      this.qaError = statusOf(err) === 409 ? 'busy' : message(err)
      return false
    }
    if (this.sessionId !== sessionId || this.disposed) return false
    const p = this.pipeline
    if (p) {
      const turns = { ...(p.qa_turns ?? {}) }
      turns[cardId] = [...(turns[cardId] ?? []).filter((n) => n !== result.turn), result.turn]
      this.pipeline = { ...p, qa_session_id: result.qa_session_id, qa_turns: turns }
    }
    this.qaPending = { cardId, turn: result.turn, question: text, answer: '' }
    void this.followQA(sessionId, result.qa_session_id)
    return true
  }

  // followQA follows the answer until its turn ends (R5): the turn may wait
  // to start (another question, a busy provider), so it keeps attaching to
  // the Q&A session's feed and reading its transcript until the question is
  // answered, or its turn ended without an answer, or qaMaxWaitMs passed.
  private async followQA(sessionId: string, qaId: string): Promise<void> {
    this.qaController?.abort()
    const controller = new AbortController()
    this.qaController = controller
    const live = () => !controller.signal.aborted && !this.disposed && this.sessionId === sessionId && !!this.qaPending
    const deadline = Date.now() + this.timing.qaMaxWaitMs
    while (live() && Date.now() < deadline) {
      let attached = false
      try {
        attached = await this.api.attachChatStream(qaId, (event) => {
          const pending = this.qaPending
          if (!pending || controller.signal.aborted) return
          if (event.type === 'delta' && event.text) this.qaPending = { ...pending, answer: pending.answer + event.text }
        }, controller.signal)
      } catch {
        attached = true
      }
      if (!live()) break
      await this.loadQA()
      const entry = this.qaPendingEntry()
      if (entry?.answer) break
      if (attached && entry && !(await this.qaRunning(qaId))) {
        // The turn ran and ended without an answer (a provider error).
        this.qaError = 'unanswered'
        break
      }
      await sleep(this.timing.qaAttachDelayMs)
    }
    if (controller.signal.aborted || this.sessionId !== sessionId || this.disposed) return
    this.qaPending = null
    if (this.qaController === controller) this.qaController = null
  }

  // qaPendingEntry is the pending question as the Q&A transcript has it.
  private qaPendingEntry(): QAEntry | undefined {
    const pending = this.qaPending
    if (!pending) return undefined
    const thread = qaThreads(this.qaHistory, this.pipeline?.qa_turns)[pending.cardId] ?? []
    return [...thread].reverse().find((e) => e.question === pending.question)
  }

  private async qaRunning(qaId: string): Promise<boolean> {
    if (!this.api.activity) return false
    try {
      const snap = await this.api.activity()
      return (snap?.running ?? []).some((r) => r.session_id === qaId)
    } catch {
      return true
    }
  }

  // dispose stops the store for good: a turn it sent still finishes on the
  // server, but nothing more is read or sent from here.
  dispose() {
    this.disposed = true
    this.controller?.abort()
    this.controller = null
    this.qaController?.abort()
    this.qaController = null
  }
}
