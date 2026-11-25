// The saved ideas as runes state: ideas kept on this phone while no desk
// answers, and sending one to a desktop as a new vibe project. Storage, the
// network, the clock and ids come in through the constructor so tests run
// it under Node.
import { APIRequestError } from '../api/client.ts'
import { easyErrorCode } from '../api/easy.ts'
import type { APITarget } from '../api/client.ts'
import type { EasyProject } from '../easy.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'
import { remoteEasyApi } from './easyRemote.ts'
import { parseIdeas, withIdea, withoutIdea, type Idea, type SentIdea } from './ideas.ts'
import type { SecureStore } from './secureStore.ts'

const storageKey = 'ideas'

// Why a send did not start a project: the desk's own code (idea_required,
// cannot_run, …), the desk revoked this phone, no answer, or anything
// else.
export type SendFailure = string

// The one call behind sending: POST /v1/easy/projects on the chosen desk.
export type IdeaAPI = {
  create(target: APITarget, idea: string): Promise<{ project: EasyProject }>
}

export const ideaAPI: IdeaAPI = {
  create: (target, idea) => remoteEasyApi(target).create(idea),
}

export type IdeaStoreDeps = {
  storage: SecureStore
  api?: IdeaAPI
  now?: () => Date
  newID?: () => string
}

export class IdeaStore {
  ideas = $state<Idea[]>([])
  loaded = $state(false)
  // The idea being sent, by id ('' when none).
  sending = $state('')
  sendFailure = $state<SendFailure | null>(null)

  #deps: IdeaStoreDeps

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

  async remove(id: string): Promise<void> {
    await this.#save(withoutIdea(this.ideas, id))
  }

  // send hands a saved idea to desktop's team as a new project, and
  // remembers where it went.
  async send(desktop: PairedDesktop, ideaID: string): Promise<SentIdea | null> {
    const idea = this.ideas.find((i) => i.id === ideaID)
    if (!idea || idea.sent || this.sending) return null
    this.sendFailure = null
    this.sending = ideaID
    try {
      const { project } = await (this.#deps.api ?? ideaAPI).create(targetOf(desktop), idea.text)
      const sent: SentIdea = { desktopID: desktop.id, desktopName: desktop.name, projectID: project.id, at: this.#now().toISOString() }
      const current = this.ideas.find((i) => i.id === ideaID) ?? idea
      await this.#save(withIdea(this.ideas, { ...current, sent }))
      return sent
    } catch (err) {
      this.sendFailure = failureOf(err)
      return null
    } finally {
      this.sending = ''
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
  return easyErrorCode(err) || 'unknown'
}
