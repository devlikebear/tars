// CASE's Bridge panel (case-companion phase 2): open/closed, the scope,
// and this page's questions and answers. The answer streams from CASE's
// own session like any turn (attachChatStream), then the final text and
// the server-checked cards come from GET /v1/case/answer. The API is
// injected so this runs under plain Node tests
// (tests/helpers/compileSvelteModule.ts).
import { caseBridgeVisibleText } from '../caseBridge.ts'
import type { askCaseBridge, attachChatStream, getCaseBridgeAnswer } from '../api'
import type { CaseBridgeCard, CaseBridgeScope, CaseBridgeView } from '../api/caseBridge.ts'

export type CaseBridgeApi = {
  askCaseBridge: typeof askCaseBridge
  getCaseBridgeAnswer: typeof getCaseBridgeAnswer
  attachChatStream: typeof attachChatStream
}

export type CaseBridgeEntryStatus = 'streaming' | 'done' | 'error'
export type CaseBridgeErrorKind = 'busy' | 'unanswered' | 'network'

export type CaseBridgeEntry = {
  id: number
  question: string
  answer: string
  cards: CaseBridgeCard[]
  status: CaseBridgeEntryStatus
  error: CaseBridgeErrorKind | null
}

export type CaseBridgeTiming = {
  // How long to wait between attaching to a turn that has not started yet
  // and asking again, and how long to keep trying in all.
  retryMs: number
  maxWaitMs: number
}

const DEFAULT_TIMING: CaseBridgeTiming = { retryMs: 400, maxWaitMs: 5 * 60_000 }

// The page keeps only the last few exchanges; CASE's session has the rest.
const MAX_ENTRIES = 20

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export class CaseBridgeStore {
  open = $state(false)
  scope = $state<CaseBridgeScope>('all')
  entries = $state<CaseBridgeEntry[]>([])

  private api: CaseBridgeApi
  private timing: CaseBridgeTiming
  private nextId = 1
  private restored = false
  private controller: AbortController | null = null

  constructor(api: CaseBridgeApi, timing: CaseBridgeTiming = DEFAULT_TIMING) {
    this.api = api
    this.timing = timing
  }

  get busy(): boolean {
    return this.entries.some((e) => e.status === 'streaming')
  }

  toggle() {
    if (this.open) this.close()
    else this.show()
  }

  // show opens the panel; the first time on this page it brings back the
  // last answer CASE gave (the server keeps it until the next question).
  show() {
    this.open = true
    if (!this.restored) {
      this.restored = true
      void this.restoreLast()
    }
  }

  close() {
    this.open = false
  }

  private async restoreLast() {
    try {
      const last = await this.api.getCaseBridgeAnswer()
      if (!last.question || this.entries.length > 0) return
      if (last.busy) {
        const entry = this.push(last.question)
        void this.follow(entry.id, last.session_id)
        return
      }
      if (!last.answer && !last.error) return
      const entry = this.push(last.question)
      this.update(entry.id, {
        answer: last.answer,
        cards: last.cards ?? [],
        status: last.error ? 'error' : 'done',
        error: last.error ? 'unanswered' : null,
      })
    } catch {
      // Nothing to restore: the panel starts empty.
    }
  }

  // ask sends a question; false when it was not sent (empty, or one is
  // still being answered) so the caller keeps the typed text.
  async ask(question: string, view: CaseBridgeView): Promise<boolean> {
    const text = question.trim()
    if (!text || this.busy) return false
    const scope = this.scope === 'screen' && view.session_id ? 'screen' : 'all'
    const entry = this.push(text)
    let sessionId = ''
    try {
      sessionId = (await this.api.askCaseBridge(text, scope, view)).session_id
    } catch (err) {
      const status = (err as { status?: number }).status
      this.update(entry.id, { status: 'error', error: status === 409 ? 'busy' : 'network' })
      return false
    }
    void this.follow(entry.id, sessionId)
    return true
  }

  // follow streams the turn, retrying while it has not started, then reads
  // the final answer and its cards.
  private async follow(entryId: number, sessionId: string) {
    this.controller?.abort()
    const controller = new AbortController()
    this.controller = controller
    const deadline = Date.now() + this.timing.maxWaitMs
    let streamed = ''
    while (!controller.signal.aborted && Date.now() < deadline) {
      try {
        await this.api.attachChatStream(
          sessionId,
          (event) => {
            if (controller.signal.aborted || event.type !== 'delta' || !event.text) return
            streamed += event.text
            this.update(entryId, { answer: caseBridgeVisibleText(streamed) })
          },
          controller.signal,
        )
      } catch {
        // A dropped stream: the answer endpoint below says where it stands.
      }
      if (controller.signal.aborted) return
      let result
      try {
        result = await this.api.getCaseBridgeAnswer()
      } catch {
        await sleep(this.timing.retryMs)
        continue
      }
      if (result.busy) {
        await sleep(this.timing.retryMs)
        continue
      }
      if (result.error || !result.answer) {
        this.update(entryId, { status: 'error', error: 'unanswered', answer: result.answer ?? '' })
      } else {
        this.update(entryId, { status: 'done', answer: result.answer, cards: result.cards ?? [] })
      }
      if (this.controller === controller) this.controller = null
      return
    }
    if (!controller.signal.aborted) this.update(entryId, { status: 'error', error: 'unanswered' })
  }

  dispose() {
    this.controller?.abort()
    this.controller = null
  }

  private push(question: string): CaseBridgeEntry {
    const entry: CaseBridgeEntry = { id: this.nextId++, question, answer: '', cards: [], status: 'streaming', error: null }
    this.entries = [...this.entries, entry].slice(-MAX_ENTRIES)
    return entry
  }

  private update(id: number, patch: Partial<CaseBridgeEntry>) {
    this.entries = this.entries.map((e) => (e.id === id ? { ...e, ...patch } : e))
  }
}
