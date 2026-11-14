// The phone's "waiting on you" inbox as runes state: ask every desktop that
// answers for its pipelines and its waiting tool calls, merge them into one
// list, and answer a tool call on the desktop that asked. The network comes
// in through the constructor so tests run it under Node.
import { APIRequestError } from '../api/client.ts'
import type { InboxAPI } from './desktopApi.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'
import { mergeInboxes, type DesktopInbox, type InboxApproval, type InboxEntry } from './inbox.ts'
import type { FocusListItem } from '../types.ts'

// Why answering a tool call did not go through.
//   gone   — already answered (at the desk, or it timed out); the list is
//            read again
//   failed — the desktop refused or did not answer
export type AnswerFailure = 'gone' | 'failed'

export class InboxStore {
  entries = $state<InboxEntry[]>([])
  // Each desktop's pipelines from the last read that answered, by desktop id.
  pipelines = $state<Record<string, FocusListItem[]>>({})
  // Desktops asked on the last read that did not answer it.
  failed = $state<string[]>([])
  loading = $state(false)
  loaded = $state(false)
  // Approval ids being answered now, and the last failure per approval id.
  answering = $state<Record<string, boolean>>({})
  answerFailures = $state<Record<string, AnswerFailure>>({})

  #api: InboxAPI
  #desks = new Map<string, DesktopInbox>()
  // The latest read per desktop: an older read that answers late is dropped.
  #reads = new Map<string, number>()
  #seq = 0

  constructor(deps: { api: InboxAPI }) {
    this.#api = deps.api
  }

  // refresh reads every desktop given (the ones that answer now) at once.
  // A desktop left out, or one that does not answer, has no entries: the
  // inbox never shows what a desktop waited on an earlier read.
  async refresh(desktops: PairedDesktop[]): Promise<void> {
    const ids = new Set(desktops.map((d) => d.id))
    for (const id of [...this.#desks.keys()]) if (!ids.has(id)) this.#desks.delete(id)
    this.failed = this.failed.filter((id) => ids.has(id))
    this.loading = true
    try {
      await Promise.all(desktops.map((d) => this.#read(d)))
    } finally {
      this.loading = false
      this.loaded = true
      this.#publish()
    }
  }

  // answer answers a tool call on the desktop that asked: approve or allow
  // once when allow, else reject or deny. The desktop is read again after.
  async answer(desktop: PairedDesktop, approval: InboxApproval, allow: boolean): Promise<boolean> {
    const target = targetOf(desktop)
    this.answering = { ...this.answering, [approval.id]: true }
    this.answerFailures = without(this.answerFailures, approval.id)
    let ok = false
    try {
      if (approval.kind === 'queued') {
        await this.#api.reviewApproval(target, approval.id, allow ? 'approve' : 'reject')
      } else {
        await this.#api.answerPermission(target, approval.id, approval.sessionID, allow ? 'allow_once' : 'deny')
      }
      ok = true
    } catch (err) {
      const gone = err instanceof APIRequestError && (err.status === 404 || err.status === 409)
      this.answerFailures = { ...this.answerFailures, [approval.id]: gone ? 'gone' : 'failed' }
    } finally {
      this.answering = without(this.answering, approval.id)
    }
    await this.#read(desktop)
    this.#publish()
    return ok
  }

  async #read(desktop: PairedDesktop): Promise<void> {
    const seq = ++this.#seq
    this.#reads.set(desktop.id, seq)
    const target = targetOf(desktop)
    try {
      const [pipelines, activity] = await Promise.all([this.#api.pipelines(target), this.#api.activity(target)])
      if (this.#reads.get(desktop.id) !== seq) return
      this.#desks.set(desktop.id, { desktopID: desktop.id, desktopName: desktop.name, pipelines: pipelines ?? [], activity: activity ?? { running: [], pending_approvals: [] } })
      this.failed = this.failed.filter((id) => id !== desktop.id)
    } catch {
      if (this.#reads.get(desktop.id) !== seq) return
      this.#desks.delete(desktop.id)
      if (!this.failed.includes(desktop.id)) this.failed = [...this.failed, desktop.id]
    }
  }

  #publish(): void {
    const desks = [...this.#desks.values()]
    this.entries = mergeInboxes(desks)
    this.pipelines = Object.fromEntries(desks.map((d) => [d.desktopID, d.pipelines]))
  }
}

function without<T>(record: Record<string, T>, key: string): Record<string, T> {
  return Object.fromEntries(Object.entries(record).filter(([k]) => k !== key))
}
