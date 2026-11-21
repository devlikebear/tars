// The app's Chat tab as runes state: the device's own chat session on one
// desktop at a time (docs/decisions/mobile-remote.md §8). Not the chat
// workbench — one session, no switching, no history beyond it. The network
// comes in through the constructor so tests run it under Node.
import { APIRequestError } from '../api/client.ts'
import type { ChatApproval } from '../chatApproval.ts'
import { withdrawPendingApprovals } from '../chatApproval.ts'
import type { ChatMessage } from '../chatMessages.ts'
import { applySideEvent, historyMessages } from '../sideSession.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'
import type { RemoteChatAPI } from './remoteChat.ts'

// Why connecting to the device's chat session failed.
//   unpaired — the desk answered 401/403: it revoked this phone's token
//   failed   — any other error (unreachable, 5xx, a bad response)
export type RemoteChatFailure = 'unpaired' | 'failed'

export class RemoteChatStore {
  messages = $state<ChatMessage[]>([])
  sessionId = $state('')
  running = $state(false)
  // True while connecting (ensuring the session, loading its history).
  loading = $state(false)
  error = $state('')
  errorKind = $state<RemoteChatFailure | ''>('')

  #api: RemoteChatAPI
  #desktop: PairedDesktop | null = null
  #controller: AbortController | null = null
  // Bumped on every connect/disconnect so a reply in flight for an earlier
  // desktop or an earlier connect never lands after a newer one started.
  #token = 0

  constructor(deps: { api: RemoteChatAPI }) {
    this.#api = deps.api
  }

  // connect switches to desktop's own chat session: ensures it exists,
  // loads its history, and attaches to a turn already running (started
  // from another tab, or still running from before this screen opened).
  async connect(desktop: PairedDesktop): Promise<void> {
    const token = ++this.#token
    this.#controller?.abort()
    this.#controller = null
    this.#desktop = desktop
    this.running = false
    this.error = ''
    this.errorKind = ''
    this.messages = []
    this.sessionId = ''
    this.loading = true
    try {
      const sessionId = await this.#api.ensureSession(targetOf(desktop))
      if (token !== this.#token) return
      this.sessionId = sessionId
      await this.#loadHistory(token)
      void this.#follow(token)
    } catch (err) {
      if (token === this.#token) {
        this.error = errorMessage(err)
        this.errorKind = err instanceof APIRequestError && (err.status === 401 || err.status === 403) ? 'unpaired' : 'failed'
      }
    } finally {
      if (token === this.#token) this.loading = false
    }
  }

  // disconnect stops following the session (the turn itself, if any, runs
  // on at the desk — leaving this screen never cancels it).
  disconnect(): void {
    this.#token++
    this.#controller?.abort()
    this.#controller = null
    this.#desktop = null
  }

  async send(text: string): Promise<void> {
    const trimmed = text.trim()
    if (!trimmed || !this.#desktop || !this.sessionId || this.running) return
    const token = this.#token
    const target = targetOf(this.#desktop)
    const sessionId = this.sessionId
    this.error = ''
    this.running = true
    this.messages = [...this.messages, { id: `remote-user-${Date.now()}`, role: 'user', text: trimmed }]
    const replyId = `remote-reply-${Date.now()}`
    const ac = new AbortController()
    this.#controller = ac
    try {
      await this.#api.send(
        target,
        sessionId,
        trimmed,
        (event) => {
          if (token !== this.#token) return
          this.messages = applySideEvent(this.messages, event, replyId)
        },
        ac.signal,
      )
    } catch (err) {
      if (!(err instanceof DOMException && err.name === 'AbortError')) this.error = errorMessage(err)
    } finally {
      if (this.#controller === ac) this.#controller = null
      if (token === this.#token) {
        this.running = false
        this.messages = withdrawPendingApprovals(this.messages)
        await this.#loadHistory(token)
      }
    }
  }

  async cancel(): Promise<void> {
    if (!this.#desktop || !this.sessionId) return
    await this.#api.cancel(targetOf(this.#desktop), this.sessionId)
  }

  async answerPermission(approval: ChatApproval, decision: 'allow_once' | 'allow_session' | 'deny'): Promise<void> {
    if (!this.#desktop) return
    const target = targetOf(this.#desktop)
    this.#updateApproval({ ...approval, state: 'sending', error: undefined })
    try {
      await this.#api.answerPermission(target, approval.requestId, approval.sessionId, decision)
    } catch {
      this.#updateApproval({ ...approval, state: 'pending', error: 'failed' })
    }
  }

  #updateApproval(next: ChatApproval): void {
    this.messages = this.messages.map((m) => (m.approval?.requestId === next.requestId ? { ...m, approval: next } : m))
  }

  async #follow(token: number): Promise<void> {
    if (!this.#desktop || this.running) return
    const ac = new AbortController()
    this.#controller = ac
    const replyId = `remote-reply-${Date.now()}`
    const target = targetOf(this.#desktop)
    const sessionId = this.sessionId
    try {
      const attached = await this.#api.attach(
        target,
        sessionId,
        (event) => {
          if (token !== this.#token) return
          this.running = true
          this.messages = applySideEvent(this.messages, event, replyId)
        },
        ac.signal,
      )
      if (!attached) return
    } catch {
      // Aborted, or the turn ended as we attached; #loadHistory below settles it.
    } finally {
      if (this.#controller === ac) this.#controller = null
      if (token === this.#token && this.running) {
        this.running = false
        this.messages = withdrawPendingApprovals(this.messages)
        await this.#loadHistory(token)
      }
    }
  }

  async #loadHistory(token: number): Promise<void> {
    if (!this.#desktop) return
    try {
      const history = await this.#api.history(targetOf(this.#desktop))
      if (token === this.#token) this.messages = historyMessages(history.messages)
    } catch {
      // Keep what is already shown.
    }
  }
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
