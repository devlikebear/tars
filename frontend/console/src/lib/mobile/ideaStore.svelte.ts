// The idea inbox as runes state: ideas kept on this phone, and sending one
// to a desktop as a focus task. Storage, the network, the clock and ids
// come in through the constructor so tests run it under Node.
import { APIRequestError } from '../api/client.ts'
import type { FocusTemplate } from '../types.ts'
import type { TaskAPI } from './desktopApi.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'
import { parseIdeas, withIdea, withoutIdea, type Idea, type RegisteredFolder, type SentIdea } from './ideas.ts'
import type { SecureStore } from './secureStore.ts'

const storageKey = 'ideas'

// Why a desktop's choices could not be read, or a send did not start a task.
//   folder      — the desk no longer lists that folder for phones
//   unavailable — the desk cannot read its registered folders
//   unpaired    — the desk revoked this phone
//   unreachable — no answer
//   failed      — any other refusal
export type SendFailure = 'folder' | 'unavailable' | 'unpaired' | 'unreachable' | 'failed'

// What one desktop offers to start a task with.
export type SendOptions = { desktopID: string; folders: RegisteredFolder[]; templates: FocusTemplate[] }

export type IdeaStoreDeps = {
  storage: SecureStore
  api: TaskAPI
  now?: () => Date
  newID?: () => string
}

export class IdeaStore {
  ideas = $state<Idea[]>([])
  loaded = $state(false)
  options = $state<SendOptions | null>(null)
  loadingOptions = $state(false)
  optionsFailure = $state<SendFailure | null>(null)
  sending = $state(false)
  sendFailure = $state<SendFailure | null>(null)

  #deps: IdeaStoreDeps
  // The latest choices read: a desktop picked and then changed answers late.
  #optionsFor = ''

  constructor(deps: IdeaStoreDeps) {
    this.#deps = deps
  }

  async load(): Promise<void> {
    this.ideas = parseIdeas(await this.#deps.storage.get(storageKey))
    this.loaded = true
  }

  // add keeps a new idea; empty text keeps nothing.
  async add(text: string): Promise<Idea | null> {
    if (!text.trim()) return null
    const idea: Idea = { id: this.#newID(), text: text.trim(), createdAt: this.#now().toISOString() }
    await this.#save(withIdea(this.ideas, idea))
    return idea
  }

  // edit changes an idea not yet sent; a sent one is its pipeline's goal now.
  async edit(id: string, text: string): Promise<void> {
    const idea = this.ideas.find((i) => i.id === id)
    if (!idea || idea.sent || !text.trim()) return
    await this.#save(withIdea(this.ideas, { ...idea, text: text.trim() }))
  }

  async remove(id: string): Promise<void> {
    await this.#save(withoutIdea(this.ideas, id))
  }

  // prepare reads what a desktop offers: the folders its owner registered
  // for phones (only those that exist now) and its templates.
  async prepare(desktop: PairedDesktop): Promise<void> {
    this.#optionsFor = desktop.id
    this.options = null
    this.optionsFailure = null
    this.loadingOptions = true
    try {
      const target = targetOf(desktop)
      const [folders, templates] = await Promise.all([this.#deps.api.folders(target), this.#deps.api.templates(target)])
      if (this.#optionsFor !== desktop.id) return
      this.options = { desktopID: desktop.id, folders: folders.filter((f) => f.exists), templates }
    } catch (err) {
      if (this.#optionsFor !== desktop.id) return
      this.optionsFailure = failureOf(err)
    } finally {
      if (this.#optionsFor === desktop.id) this.loadingOptions = false
    }
  }

  // send starts a focus task from an idea on desktop, in folder cwd, with
  // template ('' for the desk's default). goal is the idea's text as the
  // developer left it in the send sheet.
  async send(desktop: PairedDesktop, ideaID: string, goal: string, cwd: string, template: string): Promise<SentIdea | null> {
    const idea = this.ideas.find((i) => i.id === ideaID)
    if (!idea || idea.sent || !goal.trim() || !cwd) return null
    this.sendFailure = null
    this.sending = true
    try {
      const request: { goal: string; cwd: string; template?: string } = { goal: goal.trim(), cwd }
      if (template) request.template = template
      const created = await this.#deps.api.createPipeline(targetOf(desktop), request)
      const sent: SentIdea = { desktopID: desktop.id, desktopName: desktop.name, sessionID: created.session_id, at: this.#now().toISOString() }
      // The idea keeps the text the task was started with.
      const current = this.ideas.find((i) => i.id === ideaID) ?? idea
      await this.#save(withIdea(this.ideas, { ...current, text: goal.trim(), sent }))
      return sent
    } catch (err) {
      this.sendFailure = failureOf(err)
      return null
    } finally {
      this.sending = false
    }
  }

  async #save(list: Idea[]): Promise<void> {
    await this.#deps.storage.set(storageKey, JSON.stringify(list))
    this.ideas = list
  }

  #now(): Date {
    return this.#deps.now?.() ?? new Date()
  }

  #newID(): string {
    return this.#deps.newID?.() ?? crypto.randomUUID()
  }
}

function failureOf(err: unknown): SendFailure {
  if (!(err instanceof APIRequestError)) return 'unreachable'
  if (err.status === 401) return 'unpaired'
  if (err.status === 403) return 'folder'
  if (err.status === 503) return 'unavailable'
  return 'failed'
}
