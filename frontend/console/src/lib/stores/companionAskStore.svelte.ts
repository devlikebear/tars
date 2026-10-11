// CASE's bubble question-and-answer (tars#1236): one question/answer pair
// at a time, held until the user closes the bubble or asks again. The
// turn itself is a normal chat turn on the main session — pure decisions
// (tier, request shape, truncation, error classification) live in
// ../companionAsk.ts; this class is the impure shell around streamChat.
// The API is injected so this compiles and runs under plain Node tests
// (tests/helpers/compileSvelteModule.ts), same as chatSessionStore.svelte.ts.
import {
  classifyCompanionAskError,
  companionAskRequest,
  companionAskTierRecommendation,
  truncateCompanionAnswer,
  type CompanionAskErrorKind,
} from '../companionAsk.ts'
import type { listAgentRuntimeSubagents, listSessions, streamChat } from '../api'
import type { AgentRuntimeTierOption, ChatEvent, Session } from '../types'

export type CompanionAskApi = {
  listSessions: typeof listSessions
  listAgentRuntimeSubagents: typeof listAgentRuntimeSubagents
  streamChat: typeof streamChat
}

export type CompanionAskStatus = 'idle' | 'streaming' | 'done' | 'error'

export class CompanionAskStore {
  // The main session's real id ("main" only names it on the wire — the
  // console needs the real id to read its chat-activity entry and to link
  // "Continue in chat"). Resolved once and cached; EnsureMain always
  // returns the same session.
  mainSessionId = $state('')

  question = $state('')
  answer = $state('')
  truncated = $state(false)
  status = $state<CompanionAskStatus>('idle')
  errorKind = $state<CompanionAskErrorKind | null>(null)
  // True when `question` is being (or was) answered in the main session's
  // own visible thread instead of the bubble (tars#1236 requirement 5 —
  // the user is already looking at that session). Set by beginInThread/
  // finishInThread, which ChatPanel.sendCompanionMessage calls directly;
  // `answer` stays empty in this case, the bubble shows a short note
  // instead of duplicating what the thread already shows.
  inThread = $state(false)

  private api: CompanionAskApi
  private mainSessionResolving: Promise<void> | null = null
  private tierOptions: AgentRuntimeTierOption[] = []
  private defaultTier = ''
  private tierOptionsLoaded = false
  private abortController: AbortController | null = null

  constructor(api: CompanionAskApi) {
    this.api = api
  }

  get busy(): boolean {
    return this.status === 'streaming'
  }

  // Whether there is a question/answer the bubble must keep showing —
  // until the user closes it or asks again, never on its own (tars#1236
  // requirement 3).
  get hasContent(): boolean {
    return this.question !== ''
  }

  // Resolves and caches mainSessionId. Cheap to call repeatedly (e.g. on
  // every CompanionPet mount) — it only fetches once.
  async ensureMainSession(): Promise<void> {
    if (this.mainSessionId) return
    if (this.mainSessionResolving) return this.mainSessionResolving
    this.mainSessionResolving = (async () => {
      try {
        const sessions = await this.api.listSessions()
        const main = sessions.find((session) => session.kind === 'main')
        if (main) this.mainSessionId = main.id
      } catch {
        // The next call (mount, or the next ask) tries again.
      } finally {
        this.mainSessionResolving = null
      }
    })()
    return this.mainSessionResolving
  }

  private async loadTierOptionsOnce(): Promise<void> {
    if (this.tierOptionsLoaded) return
    try {
      const resp = await this.api.listAgentRuntimeSubagents()
      this.tierOptions = resp.tiers ?? []
      this.defaultTier = resp.default_tier?.trim() ?? ''
      this.tierOptionsLoaded = true
    } catch {
      // companionAskTierRecommendation with no tier options still returns
      // the light recommendation (it cannot find a CLI-kind mismatch
      // without them) — asking still works, just without the resume guard.
    }
  }

  // Clears the current question/answer — closing the bubble, or asking a
  // new question, both do this.
  reset(): void {
    this.cancel()
    this.question = ''
    this.answer = ''
    this.truncated = false
    this.status = 'idle'
    this.errorKind = null
    this.inThread = false
  }

  cancel(): void {
    this.abortController?.abort()
    this.abortController = null
  }

  // beginInThread/finishInThread let ChatPanel.sendCompanionMessage report
  // a question it is answering in its own visible thread (tars#1236
  // requirement 5) through the same status the bubble already watches,
  // without going through ask()'s streamChat call (ChatPanel makes that
  // call itself, directly into its transcript).
  beginInThread(question: string): void {
    this.cancel()
    this.question = question
    this.answer = ''
    this.truncated = false
    this.status = 'streaming'
    this.errorKind = null
    this.inThread = true
  }

  finishInThread(ok: boolean, errorKind?: CompanionAskErrorKind): void {
    this.status = ok ? 'done' : 'error'
    this.errorKind = ok ? null : errorKind ?? 'error'
  }

  // ask sends `message` as a normal chat turn to the main session.
  // `mainSessionBusy` is read from the activity poll the caller already
  // has (lib/companionAsk.ts isMainSessionBusy) so this does not need its
  // own round trip to check — a 409 is still handled below as the
  // fallback for the race where a turn started in between. Returns false
  // whenever nothing was delivered (refused, or failed before/while
  // streaming), so the caller knows to give the typed text back.
  async ask(message: string, consoleContext: string, mainSessionBusy: boolean): Promise<boolean> {
    const trimmed = message.trim()
    if (!trimmed) return false
    // Refuses outright rather than starting a second, overlapping fetch:
    // two concurrent ask() calls would both mutate question/answer/status
    // through their own handleEvent closures and corrupt each other's
    // display (#1236 review). The caller (CompanionPet) disables the ask
    // button while busy; this is the defense-in-depth backstop.
    if (this.busy) return false
    // mainSessionId is only needed here for the busy pre-check (the caller
    // already computed mainSessionBusy from it) and the tier/CLI-resume
    // lookup just below — the request itself always sends session_id
    // "main" (companionAskRequest) regardless of whether this resolved.
    // The server always has a real main session by the time it serves any
    // request (main_serve_api.go resolves it once via resolveMainSessionID/
    // EnsureMain before routes are registered), so a listSessions() hiccup
    // here must not refuse to send — only "Continue in chat" and the next
    // ask's busy check lose the id until a later listSessions() succeeds.
    await this.ensureMainSession()
    if (mainSessionBusy) {
      this.question = trimmed
      this.answer = ''
      this.truncated = false
      this.status = 'error'
      this.errorKind = 'busy'
      this.inThread = false
      return false
    }
    await this.loadTierOptionsOnce()
    let mainSession: Pick<Session, 'tier_pin' | 'upstream_session_id'> = {}
    if (this.mainSessionId) {
      try {
        const sessions = await this.api.listSessions()
        mainSession = sessions.find((session) => session.id === this.mainSessionId) ?? {}
      } catch {
        // Stale/missing tier_pin and upstream_session_id just means the CLI
        // resume guard cannot run this time; companionAskTierRecommendation
        // then returns the light recommendation as usual.
      }
    }
    const tierRecommendation = companionAskTierRecommendation(mainSession, this.tierOptions, this.defaultTier)
    const request = companionAskRequest({ message: trimmed, consoleContext, tierRecommendation })

    this.question = trimmed
    this.answer = ''
    this.truncated = false
    this.status = 'streaming'
    this.errorKind = null
    this.inThread = false
    const ac = new AbortController()
    this.abortController = ac
    try {
      await this.api.streamChat(request, (event) => this.handleEvent(event), ac.signal)
      if (this.status === 'streaming') this.status = 'done'
      // A turn on "main" just succeeded, so a session with that kind
      // definitely exists now — worth one more try if the earlier
      // ensureMainSession() came up empty, so "Continue in chat" and the
      // next ask's busy check have a real id without waiting on an
      // unrelated mount/poll.
      if (!this.mainSessionId) void this.ensureMainSession()
      return this.status === 'done'
    } catch (err) {
      if (ac.signal.aborted) {
        this.status = 'idle'
        return false
      }
      this.status = 'error'
      this.errorKind = classifyCompanionAskError(err)
      return false
    } finally {
      if (this.abortController === ac) this.abortController = null
    }
  }

  private handleEvent(event: ChatEvent): void {
    switch (event.type) {
      case 'delta': {
        if (this.truncated || !event.text) return
        const { shown, truncated } = truncateCompanionAnswer(this.answer + event.text)
        this.answer = shown
        this.truncated = truncated
        break
      }
      case 'done':
        this.status = 'done'
        break
      case 'error':
        this.status = 'error'
        this.errorKind = 'error'
        break
      default:
        break
    }
  }
}
